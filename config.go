package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"bytes"
	"path/filepath"
	"text/template"
	"time"

	_ "embed"

	"github.com/adrg/xdg"
	"github.com/caarlos0/duration"
	"github.com/caarlos0/env/v9"
	"github.com/charmbracelet/x/exp/strings"
	"github.com/muesli/termenv"
	"github.com/ollama/ollama/api"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

//go:embed config_template.yml
var configTemplate string

const (
	defaultMarkdownFormatText = "Format the response as markdown without enclosing backticks."
	defaultJSONFormatText     = "Format the response as json without enclosing backticks."
)

var help = map[string]string{
	"api":               "OpenAI compatible REST API (openai, localai, anthropic, ...)",
	"apis":              "Aliases and endpoints for OpenAI compatible REST API",
	"http-proxy":        "HTTP proxy to use for API requests",
	"model":             "Default model (gpt-3.5-turbo, gpt-4, ggml-gpt4all-j...)",
	"ask-model":         "Ask which model to use via interactive prompt",
	"max-input-chars":   "Default character limit on input to model",
	"format":            "Ask for the response to be formatted as markdown unless otherwise set",
	"format-text":       "Text to append when using the -f flag",
	"role":              "System role to use",
	"roles":             "List of predefined system messages that can be used as roles",
	"list-roles":        "List the roles defined in your configuration file",
	"prompt":            "Include the prompt from the arguments and stdin, truncate stdin to specified number of lines",
	"prompt-args":       "Include the prompt from the arguments in the response",
	"raw":               "Render output as raw text when connected to a TTY",
	"quiet":             "Quiet mode (hide the spinner while loading and stderr messages for success)",
	"help":              "Show help and exit",
	"version":           "Show version and exit",
	"max-retries":       "Maximum number of times to retry API calls",
	"no-limit":          "Turn off the client-side limit on the size of the input into the model",
	"word-wrap":         "Wrap formatted output at specific width (default is 80)",
	"max-tokens":        "Maximum number of tokens in response",
	"temp":              "Temperature (randomness) of results, from 0.0 to 2.0, -1.0 to disable",
	"stop":              "Up to 4 sequences where the API will stop generating further tokens",
	"topp":              "TopP, an alternative to temperature that narrows response, from 0.0 to 1.0, -1.0 to disable",
	"topk":              "TopK, only sample from the top K options for each subsequent token, -1 to disable",
	"fanciness":         "Your desired level of fanciness",
	"status-text":       "Text to show while generating",
	"dirs":              "Print the directories in which oi store its data",
	"reset-settings":    "Backup your old settings file and reset everything to the defaults",
	"continue":          "Continue from the last response or a given save title",
	"continue-last":     "Continue from the last response",
	"no-cache":          "Disables caching of the prompt/response",
	"title":             "Saves the current conversation with the given title",
	"list":              "Lists saved conversations",
	"delete":            "Deletes one or more saved conversations with the given titles or IDs",
	"delete-older-than": "Deletes all saved conversations older than the specified duration; valid values are " + strings.EnglishJoin(duration.ValidUnits(), true),
	"show":              "Show a saved conversation with the given title or ID",
	"theme":             "Theme to use in the forms; valid choices are charm, catppuccin, dracula, and base16",
	"show-last":         "Show the last saved conversation",
	"mcp-servers":       "MCP Servers configurations",
	"mcp-disable":       "Disable specific MCP servers",
	"mcp-list":          "List all available MCP servers",
	"mcp-list-tools":    "List all available tools from enabled MCP servers",
	"mcp-timeout":       "Timeout for MCP server calls, defaults to 15 seconds",
	"chat":              "Enter interactive chat mode (REPL)", // Add this line
}

// Model represents the LLM model used in the API call.
type Model struct {
	Name           string
	API            string
	MaxChars       int64    `yaml:"max-input-chars"`
	Aliases        []string `yaml:"aliases"`
	Fallback       string   `yaml:"fallback"`
	ThinkingBudget int      `yaml:"thinking-budget,omitempty"`
}

// API represents an API endpoint and its models.
type API struct {
	Name      string
	APIKey    string           `yaml:"api-key"`
	APIKeyEnv string           `yaml:"api-key-env"`
	APIKeyCmd string           `yaml:"api-key-cmd"`
	Version   string           `yaml:"version"` // XXX: not used anywhere
	BaseURL   string           `yaml:"base-url"`
	Models    map[string]Model `yaml:"models"`
	User      string           `yaml:"user"`
}

// APIs is a type alias to allow custom YAML decoding.
type APIs []API

// UnmarshalYAML implements sorted API YAML decoding.
func (apis *APIs) UnmarshalYAML(node *yaml.Node) error {
	for i := 0; i < len(node.Content); i += 2 {
		var api API
		if err := node.Content[i+1].Decode(&api); err != nil {
			return fmt.Errorf("error decoding YAML file: %s", err)
		}
		api.Name = node.Content[i].Value
		*apis = append(*apis, api)
	}
	return nil
}

// FormatText is a map[format]formatting_text.
type FormatText map[string]string

// UnmarshalYAML conforms with yaml.Unmarshaler.
func (ft *FormatText) UnmarshalYAML(unmarshal func(any) error) error {
	var text string
	if err := unmarshal(&text); err != nil {
		var formats map[string]string
		if err := unmarshal(&formats); err != nil {
			return err
		}
		*ft = (FormatText)(formats)
		return nil
	}

	*ft = map[string]string{
		"markdown": text,
	}
	return nil
}

// Config holds the main configuration and is mapped to the YAML settings file.
type Config struct {
	API                 string     `yaml:"default-api" env:"API"`
	Model               string     `yaml:"default-model" env:"MODEL"`
	Format              bool       `yaml:"format" env:"FORMAT"`
	FormatText          FormatText `yaml:"format-text"`
	FormatAs            string     `yaml:"format-as" env:"FORMAT_AS"`
	Raw                 bool       `yaml:"raw" env:"RAW"`
	Quiet               bool       `yaml:"quiet" env:"QUIET"`
	MaxTokens           int64      `yaml:"max-tokens" env:"MAX_TOKENS"`
	MaxCompletionTokens int64      `yaml:"max-completion-tokens" env:"MAX_COMPLETION_TOKENS"`
	MaxInputChars       int64      `yaml:"max-input-chars" env:"MAX_INPUT_CHARS"`
	Temperature         float64    `yaml:"temp" env:"TEMP"`
	Stop                []string   `yaml:"stop" env:"STOP"`
	TopP                float64    `yaml:"topp" env:"TOPP"`
	TopK                int64      `yaml:"topk" env:"TOPK"`
	NoLimit             bool       `yaml:"no-limit" env:"NO_LIMIT"`
	CachePath           string     `yaml:"cache-path" env:"CACHE_PATH"`
	NoCache             bool       `yaml:"no-cache" env:"NO_CACHE"`
	IncludePromptArgs   bool       `yaml:"include-prompt-args" env:"INCLUDE_PROMPT_ARGS"`
	IncludePrompt       int        `yaml:"include-prompt" env:"INCLUDE_PROMPT"`
	MaxRetries          int        `yaml:"max-retries" env:"MAX_RETRIES"`
	WordWrap            int        `yaml:"word-wrap" env:"WORD_WRAP"`
	Fanciness           uint       `yaml:"fanciness" env:"FANCINESS"`
	StatusText          string     `yaml:"status-text" env:"STATUS_TEXT"`
	HTTPProxy           string     `yaml:"http-proxy" env:"HTTP_PROXY"`
	APIs                APIs       `yaml:"apis"`
	System              string     `yaml:"system"`
	Role                string     `yaml:"role" env:"ROLE"`
	AskModel            bool
	Roles               map[string][]string
	ShowHelp            bool
	ResetSettings       bool
	Prefix              string
	Version             bool
	Dirs                bool
	Theme               string
	SettingsPath        string
	ContinueLast        bool
	Continue            string
	Title               string
	ShowLast            bool
	Show                string
	List                bool
	ListRoles           bool
	Delete              []string
	DeleteOlderThan     time.Duration
	User                string
	Chat                bool // Add this line

	MCPServers   map[string]MCPServerConfig `yaml:"mcp-servers"`
	MCPList      bool
	MCPListTools bool
	MCPDisable   []string
	MCPTimeout   time.Duration `yaml:"mcp-timeout" env:"MCP_TIMEOUT"`

	cacheReadFromID, cacheWriteToID, cacheWriteToTitle string
}

// MCPServerConfig holds configuration for an MCP server.
type MCPServerConfig struct {
	Type    string   `yaml:"type"`
	Command string   `yaml:"command"`
	Env     []string `yaml:"env"`
	Args    []string `yaml:"args"`
	URL     string   `yaml:"url"`
}



// UpdateConfigWithOllamaModels replaces apis -> ollama -> models with the current
// models reported by the local Ollama instance, and updates default-model only if needed:
// If default-model exists and is present in fetched models leave it.
// Otherwise set default-model to first model returned by Ollama (if any).
func UpdateConfigWithOllamaModels(configPath string, selectedModel ...string) (bool, error) {
	// Parse Ollama base URL and create client
	baseURL, err := url.Parse("http://localhost:11434")
	if err != nil {
		// This is a configuration error in the code itself, should be fatal.
		return false, modsError{err, "Failed to parse Ollama URL."}
	}
	client := api.NewClient(baseURL, http.DefaultClient)

	// Fetch models from Ollama
	ctx := context.Background()
	modelsResp, ollamaErr := client.List(ctx) // Use a new variable name

	if ollamaErr != nil {
		// CASE 1: OLLAMA NOT RUNNING
		fmt.Fprintln(os.Stderr, "Error: Could not connect to Ollama.")
		fmt.Fprintln(os.Stderr, "Please ensure Ollama is running on the default port 11434.")
		fmt.Fprintln(os.Stderr, "If it's on a different port, you may need to update your configuration.")
		// Set modelsResp to nil to signal that we should clear the models
		modelsResp = nil
	}

	if modelsResp != nil && len(modelsResp.Models) == 0 {
		// CASE 2: OLLAMA RUNNING, NO MODELS
		fmt.Fprintln(os.Stderr, "Warning: Ollama is running, but no models are installed.")
		fmt.Fprintln(os.Stderr, "Please pull a model to use, for example: 'ollama pull gemma3'")
		// We will proceed to write the config with an empty model list
	}

	// Read existing config file to compare against later
	originalData, err := os.ReadFile(configPath)
	if err != nil {
		return false, modsError{err, "Failed to read config file."}
	}

	// Unmarshal into a yaml.Node to preserve comments and structure
	var root yaml.Node
	if err := yaml.Unmarshal(originalData, &root); err != nil {
		return false, modsError{err, "Failed to parse config YAML."}
	}

	// Get the document mapping node
	var doc *yaml.Node
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		doc = root.Content[0]
	} else {
		doc = &root
	}
	if doc == nil || doc.Kind != yaml.MappingNode {
		return false, modsError{errors.New("invalid yaml root"), "Config root is not a mapping node."}
	}

	// --- Start of YAML manipulation logic ---
	var chosenDefaultModel string // **Track the chosen model**

	// helpers to work with mapping nodes
	getMapValue := func(m *yaml.Node, key string) *yaml.Node {
		if m == nil || m.Kind != yaml.MappingNode {
			return nil
		}
		for i := 0; i < len(m.Content); i += 2 {
			if k := m.Content[i]; k.Value == key {
				return m.Content[i+1]
			}
		}
		return nil
	}
	replaceMapValue := func(m *yaml.Node, key string, newVal *yaml.Node) bool {
		if m == nil || m.Kind != yaml.MappingNode {
			return false
		}
		for i := 0; i < len(m.Content); i += 2 {
			if k := m.Content[i]; k.Value == key {
				m.Content[i+1] = newVal
				return true
			}
		}
		return false
	}

	// Ensure 'apis' mapping exists
	apisNode := getMapValue(doc, "apis")
	if apisNode == nil {
		apisKey := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "apis"}
		apisVal := &yaml.Node{Kind: yaml.MappingNode}
		doc.Content = append(doc.Content, apisKey, apisVal)
		apisNode = apisVal
	}

	// Ensure 'ollama' mapping exists inside 'apis'
	ollamaNode := getMapValue(apisNode, "ollama")
	if ollamaNode == nil {
		ollamaKey := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "ollama"}
		ollamaVal := &yaml.Node{Kind: yaml.MappingNode}
		apisNode.Content = append(apisNode.Content, ollamaKey, ollamaVal)
		ollamaNode = ollamaVal
	}

	// Build a new 'models' mapping node
	newModelsNode := &yaml.Node{Kind: yaml.MappingNode}
	modelNames := make(map[string]struct{}, 0)

	// Only add models if we successfully connected AND there are models
	if modelsResp != nil {
		modelNames = make(map[string]struct{}, len(modelsResp.Models))
		for _, m := range modelsResp.Models {
			modelNames[m.Name] = struct{}{}
			nameKey := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: m.Name, Style: yaml.DoubleQuotedStyle}
			modelVal := &yaml.Node{Kind: yaml.MappingNode}
			aliasesSeq := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: m.Name}}}
			modelVal.Content = append(modelVal.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "aliases"}, aliasesSeq,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "max-input-chars"}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "650000"},
			)
			newModelsNode.Content = append(newModelsNode.Content, nameKey, modelVal)
		}
	}
	// If modelsResp is nil (Ollama down), newModelsNode will be empty, effectively clearing the list

	// Replace or add the 'models' node
	if !replaceMapValue(ollamaNode, "models", newModelsNode) {
		ollamaNode.Content = append(ollamaNode.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "models"}, newModelsNode)
	}

	// Set/update top-level 'default-model'
	// Only run default-model logic if we actually have models
	if modelsResp != nil && len(modelsResp.Models) > 0 {
		var modelToSet string
		userSelection := ""
		if len(selectedModel) > 0 {
			userSelection = selectedModel[0]
		}

		existingDefaultNode := getMapValue(doc, "default-model")

		if userSelection != "" {
			// If a model was explicitly selected, it always becomes the default.
			modelToSet = userSelection
		} else if existingDefaultNode != nil {
			// If no model was selected, check if the existing default is still valid.
			if _, ok := modelNames[existingDefaultNode.Value]; ok {
				modelToSet = existingDefaultNode.Value // It's valid, keep it.
			}
		}

		// If modelToSet is still empty, it means we need to fall back to the first model.
		if modelToSet == "" {
			modelToSet = modelsResp.Models[0].Name
		}

		// **Store the chosen model for the success message**
		chosenDefaultModel = modelToSet

		// Now, apply the change.
		if existingDefaultNode == nil {
			// Key doesn't exist, add it.
			defaultNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: modelToSet, Style: yaml.DoubleQuotedStyle}
			doc.Content = append(doc.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "default-model"}, defaultNode)
		} else {
			// Key exists, just update its value.
			existingDefaultNode.Value = modelToSet
		}
	}

	// --- End of YAML manipulation logic ---

	// Encode the modified YAML structure back to bytes
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		_ = enc.Close()
		return false, modsError{err, "Failed to encode updated YAML."}
	}
	if err := enc.Close(); err != nil {
		return false, modsError{err, "Failed to close YAML encoder."}
	}
	newData := buf.Bytes()

	// CORE LOGIC: Only write file if content has actually changed
	if bytes.Equal(originalData, newData) {
		// If Ollama wasn't running but the config already had 0 models,
		// no changes are needed, but we shouldn't print a success message.
		if ollamaErr != nil { // This 'err' is from the client.List call
			fmt.Fprintln(os.Stderr, "Ollama not running, config already reflects no models.")
		}
		return false, nil // No changes, no update needed
	}

	// Content has changed, write the new data to the config file
	if err := os.WriteFile(configPath, newData, 0o644); err != nil {
		return false, modsError{err, "Failed to write updated config."}
	}

	// **CASE 3: SUCCESS MESSAGE**
	if modelsResp == nil {
		// This is the case where Ollama was down
		fmt.Fprintln(os.Stderr, "Cleared stale Ollama models from configuration as Ollama is not running.")
	} else if chosenDefaultModel != "" {
		fmt.Fprintf(os.Stderr, "Configuration updated with %d Ollama models. Default model set to: %s\n", len(modelsResp.Models), chosenDefaultModel)
	} else {
		// This handles the case where len(modelsResp.Models) == 0
		fmt.Fprintf(os.Stderr, "Configuration updated with %d Ollama models.\n", len(modelsResp.Models))
	}

	// Signal that the update was successful
	return true, nil
}





func ensureConfig() (Config, error) {
	var c Config
	sp, err := xdg.ConfigFile(filepath.Join("oi", "oi.yml"))
	if err != nil {
		return c, modsError{err, "Could not find settings path."}
	}
	c.SettingsPath = sp

	dir := filepath.Dir(sp)
	if dirErr := os.MkdirAll(dir, 0o700); dirErr != nil { //nolint:mnd
		return c, modsError{dirErr, "Could not create cache directory."}
	}

	if dirErr := writeConfigFile(sp); dirErr != nil {
		return c, dirErr
	}
	content, err := os.ReadFile(sp)
	if err != nil {
		return c, modsError{err, "Could not read settings file."}
	}
	if err := yaml.Unmarshal(content, &c); err != nil {
		return c, modsError{err, "Could not parse settings file."}
	}

	if err := env.ParseWithOptions(&c, env.Options{Prefix: "MODS_"}); err != nil {
		return c, modsError{err, "Could not parse environment into settings file."}
	}

	if c.CachePath == "" {
		c.CachePath = filepath.Join(xdg.DataHome, "oi")
	}

	if err := os.MkdirAll(
		filepath.Join(c.CachePath, "conversations"),
		0o700,
	); err != nil { //nolint:mnd
		return c, modsError{err, "Could not create cache directory."}
	}

	if c.WordWrap == 0 {
		c.WordWrap = 80
	}

	return c, nil
}

func writeConfigFile(path string) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return createConfigFile(path)
	} else if err != nil {
		return modsError{err, "Could not stat path."}
	}
	return nil
}

func createConfigFile(path string) error {
	tmpl := template.Must(template.New("config").Parse(configTemplate))

	f, err := os.Create(path)
	if err != nil {
		return modsError{err, "Could not create configuration file."}
	}
	defer func() { _ = f.Close() }()

	m := struct {
		Config Config
		Help   map[string]string
	}{
		Config: defaultConfig(),
		Help:   help,
	}
	if err := tmpl.Execute(f, m); err != nil {
		return modsError{err, "Could not render template."}
	}
	return nil
}

func defaultConfig() Config {
	return Config{
		FormatAs: "markdown",
		FormatText: FormatText{
			"markdown": defaultMarkdownFormatText,
			"json":     defaultJSONFormatText,
		},
		MCPTimeout: 15 * time.Second,
	}
}

func useLine() string {
	appName := filepath.Base(os.Args[0])

	if stdoutRenderer().ColorProfile() == termenv.TrueColor {
		appName = makeGradientText(stdoutStyles().AppName, appName)
	}

	return fmt.Sprintf(
		"%s %s",
		appName,
		stdoutStyles().CliArgs.Render("[OPTIONS] [PREFIX TERM]"),
	)
}

func usageFunc(cmd *cobra.Command) error {
	fmt.Printf(
		"Usage:\n  %s\n\n",
		useLine(),
	)
	fmt.Println("Options:")
	cmd.Flags().VisitAll(func(f *flag.Flag) {
		if f.Hidden {
			return
		}
		if f.Shorthand == "" {
			fmt.Printf(
				"  %-44s %s\n",
				stdoutStyles().Flag.Render("--"+f.Name),
				stdoutStyles().FlagDesc.Render(f.Usage),
			)
		} else {
			fmt.Printf(
				"  %s%s %-40s %s\n",
				stdoutStyles().Flag.Render("-"+f.Shorthand),
				stdoutStyles().FlagComma,
				stdoutStyles().Flag.Render("--"+f.Name),
				stdoutStyles().FlagDesc.Render(f.Usage),
			)
		}
	})
	if cmd.HasExample() {
		fmt.Printf(
			"\nExample:\n  %s\n  %s\n",
			stdoutStyles().Comment.Render("# "+cmd.Example),
			cheapHighlighting(stdoutStyles(), examples[cmd.Example]),
		)
	}

	return nil
}