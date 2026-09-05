// Package validate carries the one error shape every service uses to say
// "this field of the request is wrong". The HTTP layer turns it into a 422
// that names the field.
package validate

// Error points at one bad field.
type Error struct {
	Field   string
	Message string
}

func (e *Error) Error() string { return e.Field + ": " + e.Message }

// Field builds an Error.
func Field(field, message string) *Error { return &Error{Field: field, Message: message} }
