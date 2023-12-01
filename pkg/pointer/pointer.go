package utils

func PAdd[T int | int64 | int32 | float64](a, b *T) *T {
	if a == nil && b == nil {
		return nil
	} else if a == nil {
		return b
	} else if b == nil {
		return a
	} else {
		v := *a + *b
		return &v
	}
}

func PSub[T int | int64 | int32 | float64](a, b *T) *T {
	if a == nil && b == nil {
		return nil
	} else if a == nil {
		v := -*b
		return &v
	} else if b == nil {
		return a
	} else {
		v := *a - *b
		return &v
	}
}

func GetPointer[T any](a T) *T {
	v := a
	return &v
}

func GetPointerOrNil[T int | int64 | int32 | string](a T) *T {
	var v T
	if a == v {
		return nil
	}
	return &a
}
