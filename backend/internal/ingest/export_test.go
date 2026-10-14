package ingest

// Loaded сообщает, была ли хотя бы одна успешная загрузка снимка (только для тестов).
func (c *KeyCache) Loaded() bool { return c.snap.Load() != nil }
