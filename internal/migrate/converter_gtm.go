package migrate

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// GTMConvertResult holds the output of converting a GTM container export.
type GTMConvertResult struct {
	Container struct {
		Tags      int `json:"tags"`
		Triggers  int `json:"triggers"`
		Variables int `json:"variables"`
	} `json:"container"`
	Warnings      []string    `json:"warnings"`
	ContainerData interface{} `json:"container_data"`
}

// gtmExport represents the top-level GTM export JSON.
type gtmExport struct {
	ExportFormatVersion int `json:"exportFormatVersion"`
	ContainerVersion    struct {
		Tag      []gtmTag      `json:"tag"`
		Trigger  []gtmTrigger  `json:"trigger"`
		Variable []gtmVariable `json:"variable"`
	} `json:"containerVersion"`
}

type gtmTag struct {
	Name              string         `json:"name"`
	Type              string         `json:"type"`
	Parameter         []gtmParameter `json:"parameter"`
	FiringTriggerID   []string       `json:"firingTriggerId"`
	BlockingTriggerID []string       `json:"blockingTriggerId"`
}

type gtmTrigger struct {
	Name              string         `json:"name"`
	Type              string         `json:"type"`
	TriggerID         string         `json:"triggerId"`
	Filter            []gtmCondition `json:"filter"`
	CustomEventFilter []gtmCondition `json:"customEventFilter"`
}

type gtmVariable struct {
	Name      string         `json:"name"`
	Type      string         `json:"type"`
	Parameter []gtmParameter `json:"parameter"`
}

type gtmParameter struct {
	Type  string `json:"type"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

type gtmCondition struct {
	Type      string         `json:"type"`
	Parameter []gtmParameter `json:"parameter"`
}

func gtmGenerateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func gtmParamValue(params []gtmParameter, key string) string {
	for _, p := range params {
		if p.Key == key {
			return p.Value
		}
	}
	return ""
}

// ConvertGTMContainer converts a Google Tag Manager container export JSON
// into the Etiquetta container import format (etiquetta_container_v1).
func ConvertGTMContainer(gtmJSON []byte) (*GTMConvertResult, error) {
	var export gtmExport
	if err := json.Unmarshal(gtmJSON, &export); err != nil {
		return nil, fmt.Errorf("invalid GTM JSON: %w", err)
	}

	result := &GTMConvertResult{}
	var warnings []string

	// --- Triggers ---
	// Build a map from GTM triggerId to new Etiquetta ID so tags can reference them.
	triggerIDMap := make(map[string]string) // GTM triggerId -> Etiquetta ID
	var etTriggers []map[string]interface{}

	for _, tr := range export.ContainerVersion.Trigger {
		etType, ok := mapTriggerType(tr.Type)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("Unsupported trigger type '%s' for '%s'", tr.Type, tr.Name))
			continue
		}

		newID := gtmGenerateID()
		triggerIDMap[tr.TriggerID] = newID

		config := map[string]interface{}{}

		if tr.Type == "customEvent" {
			// Extract event name from customEventFilter
			for _, f := range tr.CustomEventFilter {
				eventName := gtmParamValue(f.Parameter, "arg1")
				if eventName != "" {
					config["event_name"] = eventName
					break
				}
			}
		}

		etTriggers = append(etTriggers, map[string]interface{}{
			"id":           newID,
			"name":         tr.Name,
			"trigger_type": etType,
			"config":       config,
		})
	}

	// --- Variables ---
	var etVariables []map[string]interface{}

	for _, v := range export.ContainerVersion.Variable {
		etType, ok := mapVariableType(v.Type)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("Unsupported variable type '%s' for '%s'", v.Type, v.Name))
			continue
		}

		newID := gtmGenerateID()
		config := buildVariableConfig(v)

		etVariables = append(etVariables, map[string]interface{}{
			"id":            newID,
			"name":          v.Name,
			"variable_type": etType,
			"config":        config,
		})
	}

	// --- Tags ---
	var etTags []map[string]interface{}

	for _, tag := range export.ContainerVersion.Tag {
		etType, warn := mapTagType(tag)
		if warn != "" {
			warnings = append(warnings, warn)
		}
		if etType == "" {
			continue
		}

		newID := gtmGenerateID()
		config := buildTagConfig(tag, etType)

		// Map trigger IDs
		var firingIDs []string
		for _, gtmTID := range tag.FiringTriggerID {
			if etID, ok := triggerIDMap[gtmTID]; ok {
				firingIDs = append(firingIDs, etID)
			}
		}
		if firingIDs == nil {
			firingIDs = []string{}
		}

		var exceptionIDs []string
		for _, gtmTID := range tag.BlockingTriggerID {
			if etID, ok := triggerIDMap[gtmTID]; ok {
				exceptionIDs = append(exceptionIDs, etID)
			}
		}
		if exceptionIDs == nil {
			exceptionIDs = []string{}
		}

		etTags = append(etTags, map[string]interface{}{
			"id":                    newID,
			"name":                  tag.Name,
			"tag_type":              etType,
			"config":                config,
			"consent_category":      "marketing",
			"priority":              0,
			"trigger_ids":           firingIDs,
			"exception_trigger_ids": exceptionIDs,
		})
	}

	// Ensure non-nil slices for clean JSON
	if etTags == nil {
		etTags = []map[string]interface{}{}
	}
	if etTriggers == nil {
		etTriggers = []map[string]interface{}{}
	}
	if etVariables == nil {
		etVariables = []map[string]interface{}{}
	}
	if warnings == nil {
		warnings = []string{}
	}

	result.Container.Tags = len(etTags)
	result.Container.Triggers = len(etTriggers)
	result.Container.Variables = len(etVariables)
	result.Warnings = warnings
	result.ContainerData = map[string]interface{}{
		"format": "etiquetta_container_v1",
		"data": map[string]interface{}{
			"tags":      etTags,
			"triggers":  etTriggers,
			"variables": etVariables,
		},
	}

	return result, nil
}

func mapTagType(tag gtmTag) (etType string, warning string) {
	switch tag.Type {
	case "html":
		return "custom_html", ""
	case "img":
		return "custom_image", ""
	case "gaawc", "gaawe":
		return "", fmt.Sprintf("GA4 tag '%s' skipped (replaced by Etiquetta)", tag.Name)
	case "ua":
		return "", fmt.Sprintf("Universal Analytics tag '%s' skipped (deprecated)", tag.Name)
	default:
		return "", fmt.Sprintf("Unsupported tag type '%s' for '%s'", tag.Type, tag.Name)
	}
}

func buildTagConfig(tag gtmTag, etType string) map[string]interface{} {
	config := map[string]interface{}{}
	switch etType {
	case "custom_html":
		config["html"] = gtmParamValue(tag.Parameter, "html")
	case "custom_image":
		config["url"] = gtmParamValue(tag.Parameter, "url")
	}
	return config
}

func mapTriggerType(gtmType string) (string, bool) {
	switch gtmType {
	case "pageview":
		return "page_load", true
	case "domReady":
		return "dom_ready", true
	case "click", "linkClick":
		return "click_all", true
	case "customEvent":
		return "custom_event", true
	case "timer":
		return "timer", true
	case "historyChange":
		return "history_change", true
	case "formSubmission":
		return "form_submit", true
	case "elementVisibility":
		return "element_visibility", true
	default:
		return "", false
	}
}

func mapVariableType(gtmType string) (string, bool) {
	switch gtmType {
	case "jsm":
		return "js_variable", true
	case "v":
		return "data_layer", true
	case "k":
		return "cookie", true
	case "u":
		return "url_param", true
	case "c":
		return "constant", true
	default:
		return "", false
	}
}

func buildVariableConfig(v gtmVariable) map[string]interface{} {
	config := map[string]interface{}{}
	switch v.Type {
	case "jsm":
		config["javascript"] = gtmParamValue(v.Parameter, "javascript")
	case "v":
		name := gtmParamValue(v.Parameter, "name")
		if name == "" {
			name = gtmParamValue(v.Parameter, "dataLayerVersion")
		}
		config["name"] = name
	case "k":
		config["cookie_name"] = gtmParamValue(v.Parameter, "name")
	case "u":
		config["component"] = gtmParamValue(v.Parameter, "component")
	case "c":
		config["value"] = gtmParamValue(v.Parameter, "value")
	}
	return config
}
