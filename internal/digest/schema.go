package digest

// Schema is the JSON Schema passed to the agent for its answer. It mirrors
// what Parse reads; Parse still checks and cleans everything, because the
// schema constrains the agent but is not trusted.
const Schema = `{"type":"object","properties":{` +
	`"topic":{"type":"string"},` +
	`"source":{"type":"string"},` +
	`"notes":{"type":"array","items":{"type":"object","properties":{` +
	`"title":{"type":"string"},"body":{"type":"string"},"tags":{"type":"array","items":{"type":"string"}}},` +
	`"required":["title","body","tags"]}},` +
	`"actions":{"type":"array","items":{"type":"string"}},` +
	`"facts":{"type":"array","items":{"type":"string"}},` +
	`"conclusions":{"type":"array","items":{"type":"string"}},` +
	`"people":{"type":"array","items":{"type":"string"}}},` +
	`"required":["topic","source","notes","actions","facts","conclusions","people"]}`
