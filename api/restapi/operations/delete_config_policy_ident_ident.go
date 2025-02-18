// Code generated by go-swagger; DO NOT EDIT.

package operations

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the generate command

import (
	"net/http"

	"github.com/go-openapi/runtime/middleware"
)

// DeleteConfigPolicyIdentIdentHandlerFunc turns a function with the right signature into a delete config policy ident ident handler
type DeleteConfigPolicyIdentIdentHandlerFunc func(DeleteConfigPolicyIdentIdentParams) middleware.Responder

// Handle executing the request and returning a response
func (fn DeleteConfigPolicyIdentIdentHandlerFunc) Handle(params DeleteConfigPolicyIdentIdentParams) middleware.Responder {
	return fn(params)
}

// DeleteConfigPolicyIdentIdentHandler interface for that can handle valid delete config policy ident ident params
type DeleteConfigPolicyIdentIdentHandler interface {
	Handle(DeleteConfigPolicyIdentIdentParams) middleware.Responder
}

// NewDeleteConfigPolicyIdentIdent creates a new http.Handler for the delete config policy ident ident operation
func NewDeleteConfigPolicyIdentIdent(ctx *middleware.Context, handler DeleteConfigPolicyIdentIdentHandler) *DeleteConfigPolicyIdentIdent {
	return &DeleteConfigPolicyIdentIdent{Context: ctx, Handler: handler}
}

/* DeleteConfigPolicyIdentIdent swagger:route DELETE /config/policy/ident/{ident} deleteConfigPolicyIdentIdent

Delete a Policy QoS service

Delete a new Create a Policy QoS service.

*/
type DeleteConfigPolicyIdentIdent struct {
	Context *middleware.Context
	Handler DeleteConfigPolicyIdentIdentHandler
}

func (o *DeleteConfigPolicyIdentIdent) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	route, rCtx, _ := o.Context.RouteInfo(r)
	if rCtx != nil {
		*r = *rCtx
	}
	var Params = NewDeleteConfigPolicyIdentIdentParams()
	if err := o.Context.BindValidRequest(r, route, &Params); err != nil { // bind params
		o.Context.Respond(rw, r, route.Produces, route, err)
		return
	}

	res := o.Handler.Handle(Params) // actually handle the request
	o.Context.Respond(rw, r, route.Produces, route, res)

}
