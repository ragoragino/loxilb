// Code generated by go-swagger; DO NOT EDIT.

package operations

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the generate command

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"

	"github.com/loxilb-io/loxilb/api/models"
)

// GetConfigSessionulclAllHandlerFunc turns a function with the right signature into a get config sessionulcl all handler
type GetConfigSessionulclAllHandlerFunc func(GetConfigSessionulclAllParams) middleware.Responder

// Handle executing the request and returning a response
func (fn GetConfigSessionulclAllHandlerFunc) Handle(params GetConfigSessionulclAllParams) middleware.Responder {
	return fn(params)
}

// GetConfigSessionulclAllHandler interface for that can handle valid get config sessionulcl all params
type GetConfigSessionulclAllHandler interface {
	Handle(GetConfigSessionulclAllParams) middleware.Responder
}

// NewGetConfigSessionulclAll creates a new http.Handler for the get config sessionulcl all operation
func NewGetConfigSessionulclAll(ctx *middleware.Context, handler GetConfigSessionulclAllHandler) *GetConfigSessionulclAll {
	return &GetConfigSessionulclAll{Context: ctx, Handler: handler}
}

/* GetConfigSessionulclAll swagger:route GET /config/sessionulcl/all getConfigSessionulclAll

Get

Get

*/
type GetConfigSessionulclAll struct {
	Context *middleware.Context
	Handler GetConfigSessionulclAllHandler
}

func (o *GetConfigSessionulclAll) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	route, rCtx, _ := o.Context.RouteInfo(r)
	if rCtx != nil {
		*r = *rCtx
	}
	var Params = NewGetConfigSessionulclAllParams()
	if err := o.Context.BindValidRequest(r, route, &Params); err != nil { // bind params
		o.Context.Respond(rw, r, route.Produces, route, err)
		return
	}

	res := o.Handler.Handle(Params) // actually handle the request
	o.Context.Respond(rw, r, route.Produces, route, res)

}

// GetConfigSessionulclAllOKBody get config sessionulcl all o k body
//
// swagger:model GetConfigSessionulclAllOKBody
type GetConfigSessionulclAllOKBody struct {

	// ulcl attr
	UlclAttr []*models.SessionUlClEntry `json:"ulclAttr"`
}

// Validate validates this get config sessionulcl all o k body
func (o *GetConfigSessionulclAllOKBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateUlclAttr(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *GetConfigSessionulclAllOKBody) validateUlclAttr(formats strfmt.Registry) error {
	if swag.IsZero(o.UlclAttr) { // not required
		return nil
	}

	for i := 0; i < len(o.UlclAttr); i++ {
		if swag.IsZero(o.UlclAttr[i]) { // not required
			continue
		}

		if o.UlclAttr[i] != nil {
			if err := o.UlclAttr[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("getConfigSessionulclAllOK" + "." + "ulclAttr" + "." + strconv.Itoa(i))
				} else if ce, ok := err.(*errors.CompositeError); ok {
					return ce.ValidateName("getConfigSessionulclAllOK" + "." + "ulclAttr" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// ContextValidate validate this get config sessionulcl all o k body based on the context it is used
func (o *GetConfigSessionulclAllOKBody) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateUlclAttr(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *GetConfigSessionulclAllOKBody) contextValidateUlclAttr(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.UlclAttr); i++ {

		if o.UlclAttr[i] != nil {
			if err := o.UlclAttr[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("getConfigSessionulclAllOK" + "." + "ulclAttr" + "." + strconv.Itoa(i))
				} else if ce, ok := err.(*errors.CompositeError); ok {
					return ce.ValidateName("getConfigSessionulclAllOK" + "." + "ulclAttr" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// MarshalBinary interface implementation
func (o *GetConfigSessionulclAllOKBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *GetConfigSessionulclAllOKBody) UnmarshalBinary(b []byte) error {
	var res GetConfigSessionulclAllOKBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}
