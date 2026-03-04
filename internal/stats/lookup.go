package stats


// PreferID returns the ID if non-empty, otherwise falls back to the name.
// This is the canonical ID-first resolution pattern for the migration.
func PreferID(id, name string) string {
	if id != "" {
		return id
	}
	return name
}


