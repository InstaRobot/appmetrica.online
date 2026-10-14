package ingest

// Loaded сообщает, была ли хотя бы одна успешная загрузка снимка (только для тестов).
func (c *KeyCache) Loaded() bool { return c.snap.Load() != nil }

// ResolveContext открывает resolveContext для тестов порядка определения платформы.
func ResolveContext(merged ContextFields, keyPlatform string, appPlatforms []string) (ResolvedContext, bool) {
	return resolveContext(merged, keyPlatform, appPlatforms)
}
