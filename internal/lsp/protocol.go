package lsp

import "encoding/json"

const (
	MethodInitialize     = "initialize"
	MethodInitialized    = "initialized"
	MethodShutdown       = "shutdown"
	MethodExit           = "exit"
	MethodDidOpen        = "textDocument/didOpen"
	MethodDidChange      = "textDocument/didChange"
	MethodDidClose       = "textDocument/didClose"
	MethodCompletion     = "textDocument/completion"
	MethodHover          = "textDocument/hover"
	MethodDiagnostics    = "textDocument/publishDiagnostics"
	SeverityError        = 1
	SeverityWarning      = 2
	CompletionKindField  = 5
	CompletionKindModule = 9
	CompletionKindText   = 1
)

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *ResponseError  `json:"error,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  any             `json:"params,omitempty"`
}

type ResponseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type TextDocument struct {
	URI  string `json:"uri"`
	Text string `json:"text"`
}

type DidOpenParams struct {
	TextDocument TextDocument `json:"textDocument"`
}

type ContentChange struct {
	Text string `json:"text"`
}

type DidChangeParams struct {
	TextDocument   TextDocument    `json:"textDocument"`
	ContentChanges []ContentChange `json:"contentChanges"`
}

type DocumentParams struct {
	TextDocument TextDocument `json:"textDocument"`
	Position     Position     `json:"position"`
}

type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"`
	Code     string `json:"code"`
	Source   string `json:"source"`
	Message  string `json:"message"`
}

type PublishParams struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

type CompletionItem struct {
	Label         string `json:"label"`
	Kind          int    `json:"kind"`
	Detail        string `json:"detail,omitempty"`
	Documentation string `json:"documentation,omitempty"`
	InsertText    string `json:"insertText,omitempty"`
}

type Hover struct {
	Contents MarkupContent `json:"contents"`
}

type MarkupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type ServerCapabilities struct {
	TextDocumentSync   int                `json:"textDocumentSync"`
	CompletionProvider *CompletionOptions `json:"completionProvider,omitempty"`
	HoverProvider      bool               `json:"hoverProvider"`
}

type CompletionOptions struct {
	TriggerCharacters []string `json:"triggerCharacters"`
}

type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
}
