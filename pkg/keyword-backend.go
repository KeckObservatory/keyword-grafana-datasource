package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lib/pq"
	"math"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

	"database/sql"
	_ "github.com/lib/pq"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/datasource"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/backend/resource/httpadapter"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

// Get the file and line number for logging clarity
func fl() string {
	_, fileName, fileLine, ok := runtime.Caller(1)

	// Strip out the pathing information from the filename
	ss := strings.Split(fileName, "/")
	shortFileName := ss[len(ss)-1]

	var s string
	if ok {
		s = fmt.Sprintf("(%s:%d) ", shortFileName, fileLine)
	} else {
		s = ""
	}
	return s
}

// DatasourceSettings contains Postgres connection information
type DatasourceSettings struct {
	Server    string `json:"server"`
	Port      string `json:"port"`
	Role      string `json:"role"`
	Database  string `json:"database"`
	MetaTable string `json:"metatable"`
}

// Define the unit conversions, this maps onto the unitConversionOptions list in QueryEditor.tsx
const (
	UNIT_CONVERT_NONE          = iota
	UNIT_CONVERT_DEG_TO_RAD    = iota
	UNIT_CONVERT_RAD_TO_DEG    = iota
	UNIT_CONVERT_RAD_TO_ARCSEC = iota
	UNIT_CONVERT_K_TO_C        = iota
	UNIT_CONVERT_C_TO_K        = iota
)

// Define the data transforms, this maps onto the transformOptions list in QueryEditor.tsx
const (
	TRANSFORM_NONE                  = iota
	TRANSFORM_FIRST_DERIVATVE       = iota
	TRANSFORM_FIRST_DERIVATVE_1HZ   = iota
	TRANSFORM_FIRST_DERIVATVE_10HZ  = iota
	TRANSFORM_FIRST_DERIVATVE_100HZ = iota
	TRANSFORM_DELTA                 = iota
)

// LoadSettings gets the relevant settings from the plugin context
func LoadSettings(ctx backend.PluginContext) (*DatasourceSettings, error) {
	model := &DatasourceSettings{}

	settings := ctx.DataSourceInstanceSettings
	err := json.Unmarshal(settings.JSONData, &model)
	if err != nil {
		return nil, fmt.Errorf("error reading settings: %s", err.Error())
	}

	return model, nil
}

// newDatasource returns datasource.ServeOpts.
func newDatasource() datasource.ServeOpts {
	// Create an instance manager for the plugin. The function passed
	// into `NewInstanceManger` is called when the instance is created
	// for the first time or when a datasource configuration changed.
	log.DefaultLogger.Info(fl() + "Creating new keyword datasource")

	im := datasource.NewInstanceManager(newDataSourceInstance)
	ds := &KeywordDatasource{
		im: im,
	}

	mux := http.NewServeMux()
	httpResourceHandler := httpadapter.New(mux)

	// Bind the HTTP paths to functions that respond to them
	mux.HandleFunc("/services", ds.handleResourceKeywords)
	mux.HandleFunc("/keywords", ds.handleResourceKeywords)

	return datasource.ServeOpts{
		CallResourceHandler: httpResourceHandler,
		QueryDataHandler:    ds,
		CheckHealthHandler:  ds,
	}
}

type KeywordDatasource struct {
	// The instance manager can help with lifecycle management
	// of datasource instances in plugins. It's not a requirements
	// but a best practice that we recommend that you follow.
	im instancemgmt.InstanceManager
}

// QueryData handles multiple queries and returns multiple responses.
// req contains the queries []DataQuery (where each query contains RefID as a unique identifer).
// The QueryDataResponse contains a map of RefID to the response for each query, and each response
// contains Frames ([]*Frame).
func (ds *KeywordDatasource) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	log.DefaultLogger.Info(fl()+"keyword-backend.go:QueryData", "request", req)

	// create response struct
	response := backend.NewQueryDataResponse()

	// Get the configuration
	config, err := LoadSettings(req.PluginContext)
	if err != nil {
		log.DefaultLogger.Error(fl() + "settings load error")
		return nil, err
	}

	// Build the connection string
	psqlInfo := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable", config.Server, config.Port, config.Role, config.Database)

	// Open the Postgres interface
	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		log.DefaultLogger.Error(fl() + "DB connection failure")
		return nil, err
	}
	defer db.Close()

	// loop over queries and execute them individually.
	for _, q := range req.Queries {
		res := ds.query(ctx, q, db)

		// save the response in a hashmap
		// based on with RefID as identifier
		response.Responses[q.RefID] = res
	}

	return response, nil
}

type queryModel struct {
	//Datasource string `json:"datasource"`
	//DatasourceId string `json:"datasourceId"`
	Format         string `json:"format"`
	QueryText      string `json:"queryText"`
	UnitConversion int    `json:"unitConversion"`
	Transform      int    `json:"transform"`
	IntervalMs     int    `json:"intervalMs"`
	MaxDataPoints  int    `json:"maxDataPoints"`
	OrgId          int    `json:"orgId"`
	RefId          string `json:"refId"`
	Hide           bool   `json:"hide"`
}

func (ds *KeywordDatasource) query(ctx context.Context, query backend.DataQuery, db *sql.DB) backend.DataResponse {
	// Unmarshal the json into our queryModel
	var qm queryModel

	response := backend.DataResponse{}

	// Return an error if the unmarshal fails
	response.Error = json.Unmarshal(query.JSON, &qm)
	if response.Error != nil {
		return response
	}

	// Return nothing if we are hiding this keyword
	if qm.Hide {
		return response
	}

	// Create an empty data frame response and add time dimension
	empty_frame := data.NewFrame("response")
	empty_frame.Fields = append(empty_frame.Fields, data.NewField("time", nil, []time.Time{query.TimeRange.From, query.TimeRange.To}))

	// Return empty frame if query is empty
	if qm.QueryText == "" {

		// add the frames to the response
		response.Frames = append(response.Frames, empty_frame)
		return response
	}

	// Log a warning if `Format` is empty.
	if qm.Format == "" {
		log.DefaultLogger.Warn(fl() + "format is empty, defaulting to time series")
	}

	// Pick apart the keyword name from the service
	sk := strings.Split(qm.QueryText, ".")
	service := sk[0]
	keyword := sk[1]

	// Retrieve the values from the keyword archiver with Unix time as a floating point
	from_u := float64(query.TimeRange.From.UnixNano()) * 1e-9
	to_u := float64(query.TimeRange.To.UnixNano()) * 1e-9

	// Strip bad characters from the service in case of SQL injection attack
	// TODO - Is this sufficient?
	service = pq.QuoteIdentifier(service)

	// Build a SQL query for just counting
	sql_count := fmt.Sprintf("select count(time) from %s where keyword = $1 and time >= $2 and time <= $3;", service)

	// Run the query once to see how many we are going to get back
	row := db.QueryRow(sql_count, keyword, from_u, to_u)

	// Get the count value out of the query result
	var count int32
	switch err := row.Scan(&count); err {
	case sql.ErrNoRows:
		log.DefaultLogger.Error(fl() + "query no rows returned")

		// Send back an empty frame since there's no data to be had
		response.Frames = append(response.Frames, empty_frame)
		return response

	case nil:
		log.DefaultLogger.Debug(fl() + fmt.Sprintf("query yielded %d rows", count))

	default:
		log.DefaultLogger.Error(fl() + "Error from row.Scan: " + err.Error())
		// Send back an empty frame, the query failed in some way
		response.Frames = append(response.Frames, empty_frame)
		response.Error = err
		return response
	}

	// Setup and perform the query for the real data set now
	sql := fmt.Sprintf("select time, binvalue from %s where keyword = $1 and time >= $2 and time <= $3;", service)
	rows, err := db.Query(sql, keyword, from_u, to_u)

	if err != nil {
		log.DefaultLogger.Error(fl() + "query retrieval error: " + err.Error())
		response.Error = err
		return response
	}
	defer rows.Close()

	// Store times and values here first
	times := make([]time.Time, count)
	values := make([]float64, count)

	// Temporary variables for conversions/transforms
	var timetemp float64
	var valtemp, val float64
	var i int32

	// Iterate only as many rows as predicted, it's possible more rows arrived after the initial query executed!
	for i = 0; i < count; i++ {

		// Get the next row
		if rows.Next() {

			// Pull the elements out of the row
			err = rows.Scan(&timetemp, &valtemp)
			if err != nil {
				log.DefaultLogger.Error(fl() + "query scan error: " + err.Error())

				// Send back an empty frame, the query failed in some way
				response.Frames = append(response.Frames, empty_frame)
				response.Error = err
				return response
			}
		}

		// Separate the fractional seconds so we can convert it into a time.Time
		sec, dec := math.Modf(timetemp)
		times[i] = time.Unix(int64(sec), int64(dec*(1e9)))

		// If we are doing a unit conversion, perform it now while we have the single value in hand
		switch qm.UnitConversion {

		case UNIT_CONVERT_NONE:
			// No conversion, just assign it straight over
			val = valtemp

		case UNIT_CONVERT_DEG_TO_RAD:
			// RAD = DEG * π/180  (1° = 0.01745rad)
			val = valtemp * (math.Pi / 180)

		case UNIT_CONVERT_RAD_TO_DEG:
			// DEG = RAD * 180/π  (1rad = 57.296°)
			val = valtemp * (180 / math.Pi)

		case UNIT_CONVERT_RAD_TO_ARCSEC:
			// ARCSEC = RAD * (3600 * 180)/π  (1rad = 206264.806")
			val = valtemp * (3600 * 180 / math.Pi)

		case UNIT_CONVERT_K_TO_C:
			// °C = K + 273.15
			val = valtemp + 273.15

		case UNIT_CONVERT_C_TO_K:
			// K = °C − 273.15
			val = valtemp - 273.15

		default:
			// Send back an empty frame with an error, we did not understand the conversion
			response.Frames = append(response.Frames, empty_frame)
			response.Error = fmt.Errorf("Unknown unit conversion: %d", qm.UnitConversion)
			return response
		}

		// Assign the value to the result array
		values[i] = val
	}

	// Perform any requested data transforms
	switch qm.Transform {

	case TRANSFORM_NONE:
		break

	case TRANSFORM_FIRST_DERIVATVE, TRANSFORM_FIRST_DERIVATVE_1HZ, TRANSFORM_FIRST_DERIVATVE_10HZ, TRANSFORM_FIRST_DERIVATVE_100HZ:

		// Compute the first derivative of the data.
		dtimes := make([]time.Time, count-1)
		dvalues := make([]float64, count-1)

		for i = 1; i < count; i++ {
			// Calculate the dt
			dtimes[i-1] = times[i]

			// Calculate the dy/dt
			var dt, dvdt float64
			dt = (times[i].Sub(times[i-1])).Seconds()
			dvdt = (values[i] - values[i-1]) / dt

			if qm.Transform == TRANSFORM_FIRST_DERIVATVE_1HZ {
				dvdt = math.Round(dvdt)
			} else if qm.Transform == TRANSFORM_FIRST_DERIVATVE_10HZ {
				dvdt = math.Round(dvdt*10) / 10
			} else if qm.Transform == TRANSFORM_FIRST_DERIVATVE_100HZ {
				dvdt = math.Round(dvdt*100) / 100
			}

			dvalues[i-1] = dvdt
		}

		// Reassign the original arrays to be the 1st derivative results
		times = dtimes
		values = dvalues

	case TRANSFORM_DELTA:
		// Compute the deltas of the data.  This algorithm replicates what numpy diff() does in Python,
		// to the extent that it disregards the time series data.  The resultant arrays have one fewer element,
		// we drop the 0th element of time and value.  It's like a first derivative where dt is always 1.
		// See https://numpy.org/doc/stable/reference/generated/numpy.diff.html
		dtimes := make([]time.Time, count-1)
		dvalues := make([]float64, count-1)

		for i = 1; i < count; i++ {
			// Bring the time val straight across, shifted by one
			dtimes[i-1] = times[i]

			// Calculate the dx/dt and assume dt is always 1
			dvalues[i-1] = values[i] - values[i-1]
		}

		// Reassign the original arrays to be the new results
		times = dtimes
		values = dvalues

	default:
		// Send back an empty frame with an error, we did not understand the transform
		response.Frames = append(response.Frames, empty_frame)
		response.Error = fmt.Errorf("Unknown transform: %d", qm.Transform)
		return response

	}

	// Get any error encountered during iteration of the SQL result
	err = rows.Err()
	if err != nil {
		log.DefaultLogger.Error(fl() + "query row error: " + err.Error())
		response.Error = fmt.Errorf("row query error: " + err.Error())
	}

	// Start a new frame and add the times + values
	frame := data.NewFrame("response")
	frame.RefID = qm.RefId
	frame.Name = qm.QueryText

	// It looks like you can submit the values with any string for a name, which will be appended to the
	// .Name field above (thus creating a series named "service.KEYWORD values" which may not be the desired
	// name for the series.  Thus, submit it with an empty string for now which appears to work.
	//frame.Fields = append(frame.Fields, data.NewField("values", nil, values))
	frame.Fields = append(frame.Fields, data.NewField("", nil, values))
	frame.Fields = append(frame.Fields, data.NewField("time", nil, times))

	// add the frames to the response
	response.Frames = append(response.Frames, frame)

	return response
}

// CheckHealth handles health checks sent from Grafana to the plugin.
// The main use case for these health checks is the test button on the
// datasource configuration page which allows users to verify that
// a datasource is working as expected.
func (ds *KeywordDatasource) CheckHealth(ctx context.Context, req *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	var status = backend.HealthStatusOk
	var message = "Data source is working"

	config, err := LoadSettings(req.PluginContext)
	if err != nil {
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: "Invalid config",
		}, nil
	}

	// Build the connection string
	psqlInfo := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable",
		config.Server, config.Port, config.Role, config.Database)

	// See if we can open the Postgres interface
	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: "Failure to open SQL driver: " + err.Error(),
		}, nil
	}
	defer db.Close()

	// Now see if we can ping the specified database
	err = db.Ping()

	if err != nil {
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: "Failure to ping db: " + err.Error(),
		}, nil

	} else {
		// Confirmation success back to the user
		message = fmt.Sprintf("confirmed: %s:%s:%s:%s", config.Server, config.Role, config.Database, config.MetaTable)
	}

	return &backend.CheckHealthResult{
		Status:  status,
		Message: message,
	}, nil
}

func writeResult(rw http.ResponseWriter, path string, val interface{}, err error) {
	response := make(map[string]interface{})
	code := http.StatusOK
	if err != nil {
		response["error"] = err.Error()
		code = http.StatusBadRequest
	} else {
		response[path] = val
	}

	body, err := json.Marshal(response)
	if err != nil {
		body = []byte(err.Error())
		code = http.StatusInternalServerError
	}
	_, err = rw.Write(body)
	if err != nil {
		code = http.StatusInternalServerError
	}
	rw.WriteHeader(code)
}

func (ds *KeywordDatasource) handleResourceKeywords(rw http.ResponseWriter, req *http.Request) {
	log.DefaultLogger.Debug(fl() + "resource call url=" + req.URL.String() + "  method=" + req.Method)

	if req.Method != http.MethodGet {
		return
	}

	// Get the configuration
	ctx := req.Context()
	cfg, err := LoadSettings(httpadapter.PluginConfigFromContext(ctx))
	if err != nil {
		log.DefaultLogger.Error(fl() + "settings load error")
		writeResult(rw, "?", nil, err)
		return
	}

	// Build the connection string
	psqlInfo := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable", cfg.Server, cfg.Port, cfg.Role, cfg.Database)

	// See if we can open the Postgres interface
	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		log.DefaultLogger.Error(fl() + "DB connection error")
		writeResult(rw, "?", nil, err)
		return
	}
	defer db.Close()

	// Retrieve the keywords for a given service
	if strings.HasPrefix(req.URL.String(), "/keywords") {

		// The only parameter expected to come in is the one indicating for which service to retrieve the keywords
		params, err := url.ParseQuery(req.URL.RawQuery)
		if err != nil {
			log.DefaultLogger.Error(fl() + "keywords URL error: " + err.Error())
			writeResult(rw, "?", nil, err)
			return
		}
		service := params.Get("service")

		sqlStatement := "select keyword from ktlmeta where service = $1 order by keyword asc;"
		rows, err := db.Query(sqlStatement, service)

		if err != nil {
			log.DefaultLogger.Error(fl() + "keywords retrieval failure")
			writeResult(rw, "?", nil, err)
		}
		defer rows.Close()

		// Prepare a container to send back to the caller
		keywords := map[string]string{}

		// Iterate the service list and add to the return array
		var keyword string
		for rows.Next() {
			err = rows.Scan(&keyword)
			if err != nil {
				log.DefaultLogger.Error(fl() + "keywords scan error")
				writeResult(rw, "?", nil, err)
			}

			// Make a key-value pair for Grafana to use, the key is the bare keyword name and the service.keyword is the display value
			keywords[keyword] = service + "." + keyword
		}

		// get any error encountered during iteration
		err = rows.Err()
		if err != nil {
			log.DefaultLogger.Error(fl() + "services row error")
			writeResult(rw, "?", nil, err)
		}

		writeResult(rw, "keywords", keywords, err)

		// Retrieve the services list
	} else if strings.HasPrefix(req.URL.String(), "/services") {

		// Retrieve the services, all of them, 106 on 2020-06-09
		sqlStatement := "select distinct service from ktlmeta order by service ASC;"
		rows, err := db.Query(sqlStatement)

		if err != nil {
			log.DefaultLogger.Error(fl() + "services count error")
			writeResult(rw, "?", nil, err)
		}
		defer rows.Close()

		// Prepare a container to send back to the caller
		services := map[string]string{}

		// Iterate the service list and add to the return array
		var service string
		for rows.Next() {
			err = rows.Scan(&service)
			if err != nil {
				log.DefaultLogger.Error(fl() + "services scan error")
				writeResult(rw, "?", nil, err)
			}

			// Make a key-value pair for Grafana to use but the key and the value end up being the same (is this lazy?)
			services[service] = service
		}

		// get any error encountered during iteration
		err = rows.Err()
		if err != nil {
			log.DefaultLogger.Error(fl() + "services row error")
			writeResult(rw, "?", nil, err)
		}

		writeResult(rw, "services", services, err)

	} else {

		// If we got this far, it was a bogus request
		log.DefaultLogger.Error(fl() + "invalid request string")
		writeResult(rw, "?", nil, err)
	}

}

type instanceSettings struct {
	httpClient *http.Client
}

func newDataSourceInstance(setting backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	return &instanceSettings{
		httpClient: &http.Client{},
	}, nil
}

func (s *instanceSettings) Dispose() {
	// Called before creating a a new instance to allow plugin authors
	// to cleanup.
}
