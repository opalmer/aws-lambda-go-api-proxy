// Package core provides utility methods that help convert proxy events
// into an http.Request and http.ResponseWriter
package core

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"

	"github.com/aws/aws-lambda-go/events"
)

// CustomHostVariable is the name of the environment variable that contains
// the custom hostname for the request. If this variable is not set the framework
// reverts to `DefaultServerAddress`. The value for a custom host should include
// a protocol: http://my-custom.host.com
const CustomHostVariable = "GO_API_HOST"

// DefaultServerAddress is prepended to the path of each incoming reuqest
const DefaultServerAddress = "https://aws-serverless-go-api.com"

// APIGwContextHeader is the custom header key used to store the
// API Gateway context. To access the Context properties use the
// GetAPIGatewayContext method of the RequestAccessor object.
const APIGwContextHeader = "X-GoLambdaProxy-ApiGw-Context"

// TODO add constant for ALB context header

// APIGwStageVarsHeader is the custom header key used to store the
// API Gateway stage variables. To access the stage variable values
// use the GetAPIGatewayStageVars method of the RequestAccessor object.
const APIGwStageVarsHeader = "X-GoLambdaProxy-ApiGw-StageVars"

// RequestAccessor objects give access to custom API Gateway properties
// in the request.
type RequestAccessor struct {
	stripBasePath string
}

// TODO add GetALBRequestContext

// GetAPIGatewayContext extracts the API Gateway context object from a
// request's custom header.
// Returns a populated events.APIGatewayProxyRequestContext object from
// the request.
func (r *RequestAccessor) GetAPIGatewayContext(req *http.Request) (events.APIGatewayProxyRequestContext, error) {
	if req.Header.Get(APIGwContextHeader) == "" {
		return events.APIGatewayProxyRequestContext{}, errors.New("No context header in request")
	}
	context := events.APIGatewayProxyRequestContext{}
	err := json.Unmarshal([]byte(req.Header.Get(APIGwContextHeader)), &context)
	if err != nil {
		log.Println("Error while unmarshalling context")
		log.Println(err)
		return events.APIGatewayProxyRequestContext{}, err
	}
	return context, nil
}

// GetAPIGatewayStageVars extracts the API Gateway stage variables from a
// request's custom header.
// Returns a map[string]string of the stage variables and their values from
// the request.
func (r *RequestAccessor) GetAPIGatewayStageVars(req *http.Request) (map[string]string, error) {
	stageVars := make(map[string]string)
	if req.Header.Get(APIGwStageVarsHeader) == "" {
		return stageVars, errors.New("No stage vars header in request")
	}
	err := json.Unmarshal([]byte(req.Header.Get(APIGwStageVarsHeader)), &stageVars)
	if err != nil {
		log.Println("Erorr while unmarshalling stage variables")
		log.Println(err)
		return stageVars, err
	}
	return stageVars, nil
}

// StripBasePath instructs the RequestAccessor object that the given base
// path should be removed from the request path before sending it to the
// framework for routing. This is used when API Gateway is configured with
// base path mappings in custom domain names.
func (r *RequestAccessor) StripBasePath(basePath string) string {
	if strings.Trim(basePath, " ") == "" {
		r.stripBasePath = ""
		return ""
	}

	newBasePath := basePath
	if !strings.HasPrefix(newBasePath, "/") {
		newBasePath = "/" + newBasePath
	}

	if strings.HasSuffix(newBasePath, "/") {
		newBasePath = newBasePath[:len(newBasePath)-1]
	}

	r.stripBasePath = newBasePath

	return newBasePath
}

func (r *RequestAccessor) body(body string, base64encoded bool) ([]byte, error) {
	if base64encoded {
		decoded, err := base64.StdEncoding.DecodeString(body)
		if err != nil {
			return nil, err
		}
		return decoded, nil
	}
	return []byte(body), nil
}

func (r *RequestAccessor) request(body string, isbase64encoded bool, queryStringParameters map[string]string, requestPath string, method string, headers map[string]string, contextHeader string, ctxData interface{}) (*http.Request, error) {
	decodedBody, err := r.body(body, isbase64encoded)
	if err != nil {
		return nil, err
	}

	queryString := ""
	if len(queryStringParameters) > 0 {
		queryString = "?"
		queryCnt := 0
		for q := range queryStringParameters {
			if queryCnt > 0 {
				queryString += "&"
			}
			queryString += url.QueryEscape(q) + "=" + url.QueryEscape(queryStringParameters[q])
			queryCnt++
		}
	}

	path := requestPath
	if r.stripBasePath != "" && len(r.stripBasePath) > 1 {
		if strings.HasPrefix(path, r.stripBasePath) {
			path = strings.Replace(path, r.stripBasePath, "", 1)
		}
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	serverAddress := DefaultServerAddress
	if customAddress, ok := os.LookupEnv(CustomHostVariable); ok {
		serverAddress = customAddress
	}

	request, err :=  http.NewRequest(
		strings.ToUpper(method),
		serverAddress + path + queryString,
		bytes.NewReader(decodedBody),
	)
	if err != nil {
		fmt.Printf("Could not convert request %s:%s to http.Request\n", method, requestPath)
		log.Println(err)
		return nil, err
	}

	apiContext, err := json.Marshal(ctxData)
	if err != nil {
		log.Println("Could not Marshal API GW context for custom header")
		return nil, err
	}
	request.Header.Add(contextHeader, string(apiContext))

	for key, value := range headers {
		request.Header.Add(key, value)
	}

	return request, nil
}

// ProxyEventToHTTPRequest converts an API Gateway proxy events and ALB target
// group request events into an http.Request object.
// Returns the populated request with an additional two custom headers for the
// stage variables and API Gateway context. To access these properties use
// the GetAPIGatewayStageVars and GetAPIGatewayContext method of the RequestAccessor
// object.
// TODO update docs to reference ALB methods (see TODOs further up in this file)
func (r *RequestAccessor) ProxyEventToHTTPRequest(e interface{}) (*http.Request, error) {
	switch event := e.(type) {
	case events.APIGatewayProxyRequest:
		request, err := r.request(event.Body, event.IsBase64Encoded, event.QueryStringParameters, event.Path, event.HTTPMethod, event.Headers, APIGwContextHeader, event.RequestContext)
		if err != nil {
			return nil, err
		}

		stageVars, err := json.Marshal(event.StageVariables)
		if err != nil {
			log.Println("Could not marshal stage variables for custom header")
			return nil, err
		}
		request.Header.Add(APIGwStageVarsHeader, string(stageVars))
		return request, err

	case events.ALBTargetGroupRequest:
		// TODO use proper contextHeader for FIXME in function args
		return r.request(event.Body, event.IsBase64Encoded, event.QueryStringParameters, event.Path, event.HTTPMethod, event.Headers, "FIXME", event.RequestContext)

	default:
		return nil, fmt.Errorf("don't know how to handle type: %v", reflect.TypeOf(e))
	}
}
