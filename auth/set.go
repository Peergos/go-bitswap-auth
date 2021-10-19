package auth

type Set struct {
	set map[Want]struct{}
}

func NewSet() *Set {
	return &Set{set: make(map[Want]struct{})}
}

func (s *Set) Add(w Want) {
	s.set[w] = struct{}{}
}

func (s *Set) Has(w Want) bool {
	_, ok := s.set[w]
	return ok
}

func (s *Set) Remove(w Want) {
	delete(s.set, w)
}

func (s *Set) Len() int {
	return len(s.set)
}

func (s *Set) Keys() []Want {
	out := make([]Want, 0, len(s.set))
	for w := range s.set {
		out = append(out, w)
	}
	return out
}

// Visit adds a Want to the set only if it is
// not in it already.
func (s *Set) Visit(w Want) bool {
	if !s.Has(w) {
		s.Add(w)
		return true
	}

	return false
}

func (s *Set) ForEach(f func(w Want) error) error {
	for w := range s.set {
		err := f(w)
		if err != nil {
			return err
		}
	}
	return nil
}
