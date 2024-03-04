package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/datasource"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/hashicorp/golang-lru/v2/expirable"
)

// newDatasource returns datasource.ServeOpts.
func newDatasource() datasource.ServeOpts {
	// creates a instance manager for your plugin. The function passed
	// into `NewInstanceManger` is called when the instance is created
	// for the first time or when a datasource configuration changed.
	im := datasource.NewInstanceManager(newDataSourceInstance)
	ds := &SnowflakeDatasource{
		im: im,
	}

	return datasource.ServeOpts{
		QueryDataHandler:   ds,
		CheckHealthHandler: ds,
	}
}

type SnowflakeDatasource struct {
	// The instance manager can help with lifecycle management
	// of datasource instances in plugins. It's not a requirements
	// but a best practice that we recommend that you follow.
	im instancemgmt.InstanceManager
}

// QueryData handles multiple queries and returns multiple responses.
// req contains the queries []DataQuery (where each query contains RefID as a unique identifer).
// The QueryDataResponse contains a map of RefID to the response for each query, and each response
// contains Frames ([]*Frame).
func (td *SnowflakeDatasource) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {

	// create response struct
	response := backend.NewQueryDataResponse()

	password := req.PluginContext.DataSourceInstanceSettings.DecryptedSecureJSONData["password"]
	privateKey := req.PluginContext.DataSourceInstanceSettings.DecryptedSecureJSONData["privateKey"]
	im, err := td.im.Get(ctx, req.PluginContext)
	if err != nil {
		log.DefaultLogger.Error("Could not get instance for plugin", "err", err)
		return response, err
	}

	cache := im.(*instanceSettings).Cache
	config, err := getConfig(req.PluginContext.DataSourceInstanceSettings)
	if err != nil {
		log.DefaultLogger.Error("Could not get config for plugin", "err", err)
		return response, err
	}

	// loop over queries and execute them individually.
	for _, q := range req.Queries {
		// save the response in a hashmap
		// based on with RefID as identifier
		response.Responses[q.RefID] = td.query(ctx, q, config, password, privateKey, cache)
	}

	return response, nil
}

type pluginConfig struct {
	Account     string `json:"account"`
	Username    string `json:"username"`
	Role        string `json:"role"`
	Warehouse   string `json:"warehouse"`
	Database    string `json:"database"`
	Schema      string `json:"schema"`
	ExtraConfig string `json:"extraConfig"`

	CacheEnabled    bool `json:"cacheEnabled"`
	CacheMaxEntries int  `json:"cacheMaxEntries"`
	CacheTtlMinutes int  `json:"cacheTtlMinutes"`
}

func getConfig(settings *backend.DataSourceInstanceSettings) (pluginConfig, error) {
	var config pluginConfig
	err := json.Unmarshal(settings.JSONData, &config)
	if err != nil {
		return config, err
	}
	return config, nil
}

func getConnectionString(config *pluginConfig, password string, privateKey string) string {
	params := url.Values{}
	params.Add("role", config.Role)
	params.Add("warehouse", config.Warehouse)
	params.Add("database", config.Database)
	params.Add("schema", config.Schema)

	var userPass = ""
	if len(privateKey) != 0 {
		params.Add("authenticator", "SNOWFLAKE_JWT")
		params.Add("privateKey", privateKey)
		userPass = url.User(config.Username).String()
	} else {
		userPass = url.UserPassword(config.Username, password).String()
	}

	return fmt.Sprintf("%s@%s?%s&%s", userPass, config.Account, params.Encode(), config.ExtraConfig)
}

type instanceSettings struct {
	Cache *expirable.LRU[CacheKey, CacheValue]
}

func newDataSourceInstance(ctx context.Context, setting backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	log.DefaultLogger.Info("Creating instance")
	var config, err = getConfig(&setting)
	if err != nil {
		return nil, err
	}
	cache := NewCache(config.CacheMaxEntries, config.CacheTtlMinutes)
	return &instanceSettings{cache}, nil
}

func (s *instanceSettings) Dispose() {
	s.Cache.Purge()

	log.DefaultLogger.Info("Disposing of instance")
}
