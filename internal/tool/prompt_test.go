package tool

import (
	"strings"
	"testing"
)

func TestPromptFragmentEmpty(t *testing.T) {
	r := NewRegistry()
	pc := NewPromptCaller(r)
	if got := pc.PromptFragment(); got != "" {
		t.Errorf("expected empty fragment for empty registry, got %q", got)
	}
}

func TestPromptFragmentSingleTool(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{
		name: "generate_image",
		desc: "Generate an image from a text description.",
		params: []Parameter{
			{Name: "prompt", Type: "string", Description: "Image description.", Required: true},
		},
	})

	pc := NewPromptCaller(r)
	frag := pc.PromptFragment()

	if !strings.Contains(frag, "<name>generate_image</name>") {
		t.Errorf("expected tool name in fragment, got:\n%s", frag)
	}
	if !strings.Contains(frag, "<description>Generate an image from a text description.</description>") {
		t.Errorf("expected description in fragment, got:\n%s", frag)
	}
	if !strings.Contains(frag, `<parameter name="prompt" type="string" required="true">Image description.</parameter>`) {
		t.Errorf("expected parameter in fragment, got:\n%s", frag)
	}
	if !strings.Contains(frag, "<tool_use>") {
		t.Errorf("expected usage instructions in fragment, got:\n%s", frag)
	}
}

func TestPromptFragmentMultipleTools(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "alpha", desc: "Tool A"})
	r.Register(&mockTool{name: "bravo", desc: "Tool B"})

	pc := NewPromptCaller(r)
	frag := pc.PromptFragment()

	alphaIdx := strings.Index(frag, "<name>alpha</name>")
	bravoIdx := strings.Index(frag, "<name>bravo</name>")
	if alphaIdx < 0 || bravoIdx < 0 {
		t.Fatalf("expected both tools in fragment, got:\n%s", frag)
	}
	if alphaIdx > bravoIdx {
		t.Error("expected tools in alphabetical order")
	}
}

func TestParseValidToolCall(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{
		name: "generate_image",
		params: []Parameter{
			{Name: "prompt", Type: "string"},
		},
	})
	pc := NewPromptCaller(r)

	response := `Sure, I'll generate that for you.
<tool_use>
<name>generate_image</name>
<parameters>
<prompt>a cute orange cat</prompt>
</parameters>
</tool_use>`

	tc := pc.Parse(response)
	if tc == nil {
		t.Fatal("expected tool call, got nil")
	}
	if tc.Name != "generate_image" {
		t.Errorf("Name = %q, want %q", tc.Name, "generate_image")
	}
	if tc.Args["prompt"] != "a cute orange cat" {
		t.Errorf("Args[prompt] = %q, want %q", tc.Args["prompt"], "a cute orange cat")
	}
}

func TestParseMultipleParameters(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{
		name: "search",
		params: []Parameter{
			{Name: "query", Type: "string"},
			{Name: "limit", Type: "number"},
		},
	})
	pc := NewPromptCaller(r)

	response := `<tool_use>
<name>search</name>
<parameters>
<query>Go programming</query>
<limit>5</limit>
</parameters>
</tool_use>`

	tc := pc.Parse(response)
	if tc == nil {
		t.Fatal("expected tool call, got nil")
	}
	if tc.Args["query"] != "Go programming" {
		t.Errorf("Args[query] = %q, want %q", tc.Args["query"], "Go programming")
	}
	if tc.Args["limit"] != "5" {
		t.Errorf("Args[limit] = %q, want %q", tc.Args["limit"], "5")
	}
}

func TestParseNoToolCall(t *testing.T) {
	r := NewRegistry()
	pc := NewPromptCaller(r)

	tc := pc.Parse("Just a regular text response.")
	if tc != nil {
		t.Errorf("expected nil for non-tool response, got %+v", tc)
	}
}

func TestParseUnknownTool(t *testing.T) {
	r := NewRegistry()
	pc := NewPromptCaller(r)

	response := `<tool_use>
<name>unknown_tool</name>
<parameters>
<arg>value</arg>
</parameters>
</tool_use>`

	tc := pc.Parse(response)
	if tc != nil {
		t.Errorf("expected nil for unknown tool, got %+v", tc)
	}
}

func TestParseEmptyParameters(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "ping"})
	pc := NewPromptCaller(r)

	response := `<tool_use>
<name>ping</name>
<parameters>
</parameters>
</tool_use>`

	tc := pc.Parse(response)
	if tc == nil {
		t.Fatal("expected tool call, got nil")
	}
	if tc.Name != "ping" {
		t.Errorf("Name = %q, want %q", tc.Name, "ping")
	}
	if len(tc.Args) != 0 {
		t.Errorf("expected empty args, got %v", tc.Args)
	}
}

func TestRoundTrip(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{
		name: "generate_image",
		desc: "Generate an image.",
		params: []Parameter{
			{Name: "prompt", Type: "string", Description: "Image description.", Required: true},
		},
	})
	pc := NewPromptCaller(r)

	frag := pc.PromptFragment()
	if !strings.Contains(frag, "<tool_use>") {
		t.Fatal("fragment missing usage instructions")
	}

	response := `<tool_use>
<name>generate_image</name>
<parameters>
<prompt>a sunset over mountains</prompt>
</parameters>
</tool_use>`

	tc := pc.Parse(response)
	if tc == nil {
		t.Fatal("round-trip failed: Parse returned nil")
	}
	if tc.Name != "generate_image" {
		t.Errorf("round-trip Name = %q, want %q", tc.Name, "generate_image")
	}
	if tc.Args["prompt"] != "a sunset over mountains" {
		t.Errorf("round-trip Args[prompt] = %q, want %q", tc.Args["prompt"], "a sunset over mountains")
	}
}
