package server

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connect wires an in-memory client to a built server.
func connect(t *testing.T, enableWrites bool) (*mcp.ClientSession, context.Context) {
	t.Helper()
	t.Setenv("META_GRAPH_ORIGIN", "")
	t.Setenv("META_ALLOW_GRAPH_ORIGIN_OVERRIDE", "")

	built, err := Build("test-token", enableWrites)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		_ = built.Run(ctx, serverTransport)
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "conformance-shaped-test", Version: "v0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() {
		_ = session.Close()
		<-serverDone
	})
	return session, ctx
}

// THE ASSUMPTION THIS TEST EXISTS TO PIN.
//
// The conformance runner reads result.content[0].text and JSON-parses it. The
// SDK's typed-handler form populates StructuredContent and fills Content from
// the output value; this server sets Content explicitly instead. If the SDK ever
// changes how Content is produced, this fails by name rather than ten
// conformance cases failing with a confusing cause.
func TestToolResultCarriesParseableJSONInTheFirstTextContent(t *testing.T) {
	session, ctx := connect(t, false)

	// A target that cannot reach Graph still produces an answer — an error
	// object — and that answer must be shaped like every other one.
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "comment_activity",
		Arguments: map[string]any{"page": "pg1", "post": "pg1_p1"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	if len(res.Content) == 0 {
		t.Fatal("result carried no content; the conformance runner reads content[0].text")
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] is %T, want *mcp.TextContent", res.Content[0])
	}
	if text.Text == "" {
		t.Fatal("content[0].text is empty")
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(text.Text), &parsed); err != nil {
		t.Fatalf("content[0].text does not parse as JSON: %v\ntext: %s", err, text.Text)
	}
}

// A refusal is an answer. Two targets must come back as a readable error in the
// same envelope as everything else, not as a protocol failure.
func TestARefusalArrivesAsAReadableAnswer(t *testing.T) {
	session, ctx := connect(t, false)

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "comment_activity",
		Arguments: map[string]any{"page": "pg1", "post": "pg1_p1"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	var parsed map[string]any
	_ = json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &parsed)
	message, ok := parsed["error"].(string)
	if !ok {
		t.Fatalf("no error key in the answer: %+v", parsed)
	}
	if message == "" {
		t.Fatal("the error message is empty")
	}
}

// One of the two guards on pageId. Removing it alone leaves the rule satisfied
// by the body guard — but it should be here.
func TestRespondToCommentMarksPageIDRequiredInItsSchema(t *testing.T) {
	session, ctx := connect(t, true)

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	for _, tool := range tools.Tools {
		if tool.Name != "respond_to_comment" {
			continue
		}
		encoded, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("marshal schema: %v", err)
		}
		var schema struct {
			Required []string `json:"required"`
		}
		if err := json.Unmarshal(encoded, &schema); err != nil {
			t.Fatalf("unmarshal schema: %v", err)
		}
		for _, field := range []string{"commentId", "message", "pageId"} {
			if !slices.Contains(schema.Required, field) {
				t.Errorf("%s is not required in the schema; required = %v", field, schema.Required)
			}
		}
		return
	}
	t.Fatal("respond_to_comment was not registered")
}

// A server built without writes does not advertise them, so a model cannot call
// something it will only be refused.
func TestWriteToolsAreAbsentWhenWritesAreDisabled(t *testing.T) {
	session, ctx := connect(t, false)

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	names := []string{}
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}

	if !slices.Contains(names, "comment_activity") {
		t.Errorf("comment_activity is missing; tools = %v", names)
	}
	for _, write := range []string{"respond_to_comment", "moderate_comment"} {
		if slices.Contains(names, write) {
			t.Errorf("%s is advertised with writes disabled; tools = %v", write, names)
		}
	}
}

func TestWriteToolsArePresentWhenWritesAreEnabled(t *testing.T) {
	session, ctx := connect(t, true)

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	names := []string{}
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	for _, want := range []string{"comment_activity", "respond_to_comment", "moderate_comment"} {
		if !slices.Contains(names, want) {
			t.Errorf("%s is missing; tools = %v", want, names)
		}
	}
}

// comment_activity takes no required argument: the no-target call IS the
// orientation rung, and a schema that demanded a target would make the cheap
// first call impossible.
func TestCommentActivityRequiresNoTarget(t *testing.T) {
	session, ctx := connect(t, false)

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range tools.Tools {
		if tool.Name != "comment_activity" {
			continue
		}
		encoded, _ := json.Marshal(tool.InputSchema)
		var schema struct {
			Required []string `json:"required"`
		}
		_ = json.Unmarshal(encoded, &schema)
		if len(schema.Required) != 0 {
			t.Fatalf("comment_activity requires %v; the no-target call is the orientation rung", schema.Required)
		}
		return
	}
	t.Fatal("comment_activity was not registered")
}

// An origin set without the opt-in must stop the server before it starts, not be
// ignored — a server that quietly talked to real Graph while a test believed it
// was talking to a stub is the failure the two-variable rule exists to prevent.
func TestBuildRefusesAnOriginWithoutTheOptIn(t *testing.T) {
	t.Setenv("META_GRAPH_ORIGIN", "http://127.0.0.1:9")
	t.Setenv("META_ALLOW_GRAPH_ORIGIN_OVERRIDE", "")

	if _, err := Build("test-token", false); err == nil {
		t.Fatal("Build accepted an origin override with no opt-in")
	}
}
