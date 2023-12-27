package fp

func Includes[T comparable](item T, items []T) bool {
	for _, i := range items {
		if item == i {
			return true
		}
	}

	return false
}
