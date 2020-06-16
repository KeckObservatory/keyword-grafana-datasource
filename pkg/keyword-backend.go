package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lib/pq"
	"math"
	"net/http"
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

// Define the unit conversion transforms, this maps onto the unitConversionOptions list in QueryEditor.tsx
const (
	UNIT_CONVERT_NONE       = iota
	UNIT_CONVERT_DEG_TO_RAD = iota
	UNIT_CONVERT_RAD_TO_DEG = iota
	UNIT_CONVERT_K_TO_C     = iota
	UNIT_CONVERT_C_TO_K     = iota
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

	// Enable line numbers in logging
	log.DefaultLogger.Info(fl() + "Creating new keyword datasource")

	im := datasource.NewInstanceManager(newDataSourceInstance)
	ds := &KeywordDatasource{
		im: im,
	}

	mux := http.NewServeMux()
	httpResourceHandler := httpadapter.New(mux)

	// Bind the HTTP paths to functions that respond to them
	mux.HandleFunc("/keywords", ds.handleResourceKeywords)
	mux.HandleFunc("/services", ds.handleResourceKeywords)

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
func (td *KeywordDatasource) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
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
		res := td.query(ctx, q, db)

		// save the response in a hashmap
		// based on with RefID as identifier
		response.Responses[q.RefID] = res
	}

	return response, nil
}

type queryModel struct {
	//Constant string `json:"constant"`
	//Datasource string `json:"datasource"`
	//DatasourceId string `json:"datasourceId"`
	Format         string `json:"format"`
	QueryText      string `json:"queryText"`
	UnitConversion int    `json:"unitConversion"`
	IntervalMs     int    `json:"intervalMs"`
	MaxDataPoints  int    `json:"maxDataPoints"`
	//OrgId string `json:"orgId"`
	//RefId string `json:"refId"`
}

func (td *KeywordDatasource) query(ctx context.Context, query backend.DataQuery, db *sql.DB) backend.DataResponse {
	// Unmarshal the json into our queryModel
	var qm queryModel

	response := backend.DataResponse{}

	// Return an error if the unmarshal fails
	response.Error = json.Unmarshal(query.JSON, &qm)
	if response.Error != nil {
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

	var tf float64
	var tv, v float64
	var i int32

	// Iterate only as many rows as predicted, it's possible more rows arrived after the initial query executed!
	for i = 0; i < count; i++ {

		// Get the next row
		if rows.Next() {

			// Pull the elements out of the row
			err = rows.Scan(&tf, &tv)
			if err != nil {
				log.DefaultLogger.Error(fl() + "query scan error: " + err.Error())

				// Send back an empty frame, the query failed in some way
				response.Frames = append(response.Frames, empty_frame)
				response.Error = err
				return response
			}
		}

		// Separate the fractional seconds so we can convert it into a time.Time
		sec, dec := math.Modf(tf)
		times[i] = time.Unix(int64(sec), int64(dec*(1e9)))

		// If we are doing a unit conversion, perform it now while we have the single value in hand
		switch qm.UnitConversion {

		case UNIT_CONVERT_NONE:
			// No conversion, just assign it straight over
			v = tv

		case UNIT_CONVERT_DEG_TO_RAD:
			// RAD = DEG * π/180  (1° = 0.01745rad)
			v = tv * (math.Pi / 180)

		case UNIT_CONVERT_RAD_TO_DEG:
			// DEG = RAD * 180/π  (1rad = 57.296°)
			v = tv * (180 / math.Pi)

		case UNIT_CONVERT_K_TO_C:
			// °C = K + 273.15
			v = tv + 273.15

		case UNIT_CONVERT_C_TO_K:
			// K = °C − 273.15
			v = tv - 273.15

		default:
			// Send back an empty frame with an error, we did not understand the conversion
			response.Frames = append(response.Frames, empty_frame)
			response.Error = fmt.Errorf("Unknown unit conversion: %d", qm.UnitConversion)
			return response
		}

		values[i] = v

	}

	// get any error encountered during iteration
	err = rows.Err()
	if err != nil {
		log.DefaultLogger.Error(fl() + "query row error: " + err.Error())
		response.Error = fmt.Errorf("row query error: " + err.Error())
	}

	// Start a new frame and add the times + values
	frame := data.NewFrame("response")
	frame.Fields = append(frame.Fields, data.NewField("values", nil, values))
	frame.Fields = append(frame.Fields, data.NewField("time", nil, times))

	// add the frames to the response
	response.Frames = append(response.Frames, frame)

	return response
}

// CheckHealth handles health checks sent from Grafana to the plugin.
// The main use case for these health checks is the test button on the
// datasource configuration page which allows users to verify that
// a datasource is working as expected.
func (td *KeywordDatasource) CheckHealth(ctx context.Context, req *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
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
		service := strings.Split(req.URL.RawQuery, "=")[1]

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
	// Called before creatinga a new instance to allow plugin authors
	// to cleanup.
}
