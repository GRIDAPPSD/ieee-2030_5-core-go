package sep2

// clonePtr returns a pointer to a copy of *p, or nil when p is nil. It is the
// one place a scalar or flat-struct pointer field is cloned; types that hold
// pointers inside the pointee call that type's own Copy instead.
func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
