package gorillamux

import (
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/awslabs/aws-lambda-go-api-proxy/core"
	"github.com/gorilla/mux"
)

type GorillaMuxAdapter struct {
	core.RequestAccessor
	router *mux.Router
}

func New(router *mux.Router) *GorillaMuxAdapter {
	return &GorillaMuxAdapter{
		router: router,
	}
}

func (h *GorillaMuxAdapter) Proxy(event interface{}) (interface{}, error) {
	// Lambda event -> HTTP request
	req, err := h.ProxyEventToHTTPRequest(event)
	if err != nil {
		switch event.(type) {
		case events.APIGatewayProxyRequest:
			return core.GatewayTimeout(), core.NewLoggedError("Could not convert proxy event to request: %v", err)
		case events.ALBTargetGroupRequest:
			// TODO
			return nil, nil
		default:
			// TODO
			return nil, nil
		}
	}

	// HTTP Request -> Framework
	w := core.NewProxyResponseWriter()
	h.router.ServeHTTP(http.ResponseWriter(w), req)

	// HTTP Response -> Lambda response
	switch event.(type) {
	case events.APIGatewayProxyRequest:
		resp, err := w.GetProxyResponse()
		if err != nil {
			return core.GatewayTimeout(), core.NewLoggedError("Error while generating proxy response: %v", err)
		}
		return resp, err
	case events.ALBTargetGroupRequest:
		// TODO
		return nil, nil
	default:
		// TODO
		return nil, nil
	}
}
