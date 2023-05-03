package set

type Set[T comparable] struct{ m map[T]struct{} }

func FromSlice[T comparable](slice []T) Set[T] {
	m := make(map[T]struct{}, len(slice))
	for _, v := range slice {
		m[v] = struct{}{}
	}

	return Set[T]{m}
}

func (s *Set[T]) Has(e T) bool {
	if s == nil || s.m == nil {
		return false
	}
	_, has := s.m[e]
	return has
}
