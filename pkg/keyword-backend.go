package main

import (
	"context"
	"encoding/json"
	"fmt"
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
	var s string
	if ok {
		s = fmt.Sprintf("(%s:%d)", fileName, fileLine)
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
		log.DefaultLogger.Info(fl() + "settings load error")
		return nil, err
	}

	// Build the connection string
	psqlInfo := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable", config.Server, config.Port, config.Role, config.Database)

	// Open the Postgres interface
	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		log.DefaultLogger.Info(fl() + "DB connection failure")
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
	Format        string `json:"format"`
	QueryText     string `json:"queryText"`
	IntervalMs    int    `json:"intervalMs"`
	MaxDataPoints int    `json:"maxDataPoints"`
	//OrgId string `json:"orgId"`
	//QueryText string `json:"queryText"`
	//RefId string `json:"refId"`
}

func (td *KeywordDatasource) query(ctx context.Context, query backend.DataQuery, db *sql.DB) backend.DataResponse {
	// Unmarshal the json into our queryModel
	var qm queryModel

	response := backend.DataResponse{}

	response.Error = json.Unmarshal(query.JSON, &qm)
	if response.Error != nil {
		return response
	}

	// Create an empty data frame response and add time dimension
	frame := data.NewFrame("response")
	frame.Fields = append(frame.Fields, data.NewField("time", nil, []time.Time{query.TimeRange.From, query.TimeRange.To}))

	// Return empty frame if query is empty
	if qm.QueryText == "" {

		// add the frames to the response
		response.Frames = append(response.Frames, frame)
		return response
	}

	// Log a warning if `Format` is empty.
	if qm.Format == "" {
		log.DefaultLogger.Warn(fl() + "format is empty, defaulting to time series")
	}

	// Retrieve the values from the archiver

	// Add the values
	frame.Fields = append(frame.Fields,
		data.NewField("values", nil, []int64{1, 31}),
	)

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
		log.DefaultLogger.Info(fl() + "settings load error")
		writeResult(rw, "?", nil, err)
		return
	}

	// Build the connection string
	psqlInfo := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable", cfg.Server, cfg.Port, cfg.Role, cfg.Database)

	// See if we can open the Postgres interface
	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		log.DefaultLogger.Info(fl() + "DB connection error")
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
			log.DefaultLogger.Info(fl() + "keywords retrieval failure")
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
				log.DefaultLogger.Info(fl() + "keywords scan error")
				writeResult(rw, "?", nil, err)
			}

			// Make a key-value pair for Grafana to use, the key is the bare keyword name and the service.keyword is the display value
			keywords[keyword] = service + "." + keyword
		}

		// get any error encountered during iteration
		err = rows.Err()
		if err != nil {
			log.DefaultLogger.Info(fl() + "services row error")
			writeResult(rw, "?", nil, err)
		}

		writeResult(rw, "keywords", keywords, err)

		// Retrieve the services list
	} else if strings.HasPrefix(req.URL.String(), "/services") {

		// Retrieve the services, all of them, 106 on 2020-06-09
		sqlStatement := "select distinct service from ktlmeta order by service ASC;"
		rows, err := db.Query(sqlStatement)

		if err != nil {
			log.DefaultLogger.Info(fl() + "services count error")
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
				log.DefaultLogger.Info(fl() + "services scan error")
				writeResult(rw, "?", nil, err)
			}

			// Make a key-value pair for Grafana to use but the key and the value end up being the same (is this lazy?)
			services[service] = service
		}

		// get any error encountered during iteration
		err = rows.Err()
		if err != nil {
			log.DefaultLogger.Info(fl() + "services row error")
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
