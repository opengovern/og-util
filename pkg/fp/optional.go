package fp

// Optional allows for generic creation of pointers to values.
func Optional[T any](value T) *T {
	return &value
}
