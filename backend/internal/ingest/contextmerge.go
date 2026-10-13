package ingest

// ContextFields — заданные поля EventContext.
type ContextFields map[string]string

// ResolvedContext — итоговые колонки контекста события.
type ResolvedContext struct {
	Platform, OSName, OSVersion, AppVersion, AppBuild, DeviceModel, DeviceType, Locale, Environment string
	Country, Region, City                                                                           string
}

// MergeContext сливает context батча и события.
func MergeContext(_, _ ContextFields) ContextFields { return nil }
