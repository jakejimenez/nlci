package router

// builtinSynonymMap returns the default synonym dictionary.
// Keys are intent words; values are subcommand names they map to.
func builtinSynonymMap() map[string][]string {
	return map[string][]string{
		// Listing / viewing
		"list":     {"ls", "ps", "get", "list"},
		"show":     {"get", "describe", "inspect", "ps", "show", "list"},
		"view":     {"get", "describe", "inspect", "show"},
		"display":  {"get", "describe", "inspect", "show"},
		"find":     {"get", "search", "query", "find"},
		"search":   {"search", "find", "query"},
		"describe": {"describe", "inspect", "info"},
		"info":     {"info", "describe", "inspect"},
		"status":   {"status", "ps", "get"},
		"check":    {"status", "inspect", "get"},

		// Creation / running
		"create":  {"create", "run", "new", "add", "make"},
		"new":     {"create", "new", "run", "add"},
		"run":     {"run", "exec", "start"},
		"start":   {"start", "run"},
		"launch":  {"run", "start"},
		"deploy":  {"deploy", "apply", "run"},
		"apply":   {"apply", "deploy"},
		"install": {"install", "add"},
		"add":     {"add", "create", "install"},
		"build":   {"build"},
		"make":    {"build", "create"},

		// Deletion / stopping
		"delete":  {"rm", "delete", "remove", "del"},
		"remove":  {"rm", "remove", "delete", "del"},
		"rm":      {"rm", "remove", "delete"},
		"stop":    {"stop", "kill", "down"},
		"kill":    {"kill", "stop"},
		"down":    {"down", "stop"},
		"destroy": {"destroy", "delete", "rm"},
		"drop":    {"drop", "delete", "rm"},

		// Cleaning
		"clean":   {"prune", "system", "rm", "clean"},
		"cleanup": {"prune", "system", "clean"},
		"prune":   {"prune"},
		"purge":   {"prune", "purge", "rm"},

		// Entry / access
		"enter":  {"exec", "attach", "shell", "ssh"},
		"exec":   {"exec"},
		"attach": {"attach", "exec"},
		"shell":  {"exec", "shell"},
		"ssh":    {"exec", "ssh"},
		"access": {"exec", "attach"},
		"into":   {"exec", "attach"},

		// Monitoring / observation
		"watch":   {"stats", "events", "logs", "watch"},
		"monitor": {"stats", "monitor", "watch"},
		"logs":    {"logs", "log"},
		"log":     {"logs", "log"},
		"stats":   {"stats"},
		"events":  {"events"},
		"top":     {"top", "stats"},

		// Networking / transfer
		"pull":  {"pull", "fetch", "download"},
		"push":  {"push", "upload"},
		"fetch": {"fetch", "pull"},
		"sync":  {"sync", "pull", "push"},

		// Restart / reload
		"restart": {"restart"},
		"reload":  {"reload", "restart"},
		"reset":   {"reset", "restart"},
		"bounce":  {"restart"},

		// Scaling
		"scale":  {"scale"},
		"resize": {"scale"},

		// Configuration
		"config":    {"config", "configure", "set"},
		"configure": {"configure", "config"},
		"set":       {"set", "config"},
		"update":    {"update", "set", "patch"},
		"patch":     {"patch", "update"},
		"edit":      {"edit", "update", "patch"},

		// Context / environment
		"use":    {"use", "switch"},
		"switch": {"switch", "use"},

		// Copy / move
		"copy": {"cp", "copy"},
		"cp":   {"cp"},
		"move": {"mv", "move"},
		"mv":   {"mv"},
	}
}
