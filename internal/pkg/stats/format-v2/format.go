package formatv2

type Stats struct {
	Summary   Summary             `json:"summary"`
	Config    Resource            `json:"config"`
	Resources map[string]Resource `json:"summary"`
	Functions map[string]Function `json:"functions"`
}

type Summary struct {
	ResourceCount            int `json:"resourceCount"`
	ResourceDescriptionCount int `json:"resourceDescriptionCount"`

	FunctionCount            int `json:"functionCount"`
	FunctionDescriptionCount int `json:"functionDescriptionCount"`

	PropertyCount            int `json:"propertyCount"`
	PropertyDescriptionCount int `json:"propertyDescriptionCount"`
}

type Object struct {
	Properties map[string]Property `json:"properties"`
}

type Resource struct {
	Object

	Input Object `json:"input"`

	State Object `json:"state"`
}

type Property struct {
	HasDescription bool `json:"hasDescription"`
}

type Function struct {
	Object

	Input Object `json:"input"`
}
