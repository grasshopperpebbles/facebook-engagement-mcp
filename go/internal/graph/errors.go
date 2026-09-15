package graph

import "fmt"

// graphErrorBody is the shape of the "error" object in a Graph error response.
type graphErrorBody struct {
	Message      *string `json:"message"`
	Type         *string `json:"type"`
	Code         *int    `json:"code"`
	ErrorSubcode *int    `json:"error_subcode"`
	FbtraceID    *string `json:"fbtrace_id"`
}

// MetaAPIError is how every failure from this client surfaces.
//
// The access token never appears in the message or in any field: errors reach
// logs and model context routinely, and a token in either is a credential leak.
//
// Pointers rather than zero values throughout, for the same reason the raw Graph
// types use them — a Code of 0 and an absent Code are different facts, and the
// classification below must not treat "Graph said nothing" as "Graph said zero".
type MetaAPIError struct {
	Message string
	Status  *int
	Code    *int
	Subcode *int
	// FbtraceID is Meta's request identifier. Quote it when escalating to Meta.
	FbtraceID      *string
	Type           *string
	IsNetworkError bool
}

func (e *MetaAPIError) Error() string {
	if e.Code != nil {
		return fmt.Sprintf("%s (code %d)", e.Message, *e.Code)
	}
	return e.Message
}

// IsThrottled reports whether Meta asked us to back off.
func (e *MetaAPIError) IsThrottled() bool {
	return e.Code != nil && throttleErrorCodes[*e.Code]
}

// IsAuthError reports a token that is invalid or expired — re-authentication,
// not a retry.
func (e *MetaAPIError) IsAuthError() bool {
	return (e.Code != nil && *e.Code == 190) || (e.Status != nil && *e.Status == 401)
}

// IsRetryable reports a failure worth trying again after a delay: throttling, a
// network fault, or a transient upstream 5xx. Reads consult this; writes never
// do, because a retried reply is a double post.
func (e *MetaAPIError) IsRetryable() bool {
	return e.IsThrottled() || e.IsNetworkError || (e.Status != nil && *e.Status >= 500)
}
