package island

// TEMPORARY until the config work lands: [plugins.<name>] enabled = auto.
func (c Config) PluginOn(name string, detected bool) bool {
	if name == "edge" && !c.Edge {
		return false
	}
	return detected
}
