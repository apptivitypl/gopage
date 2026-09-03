package lsp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

type Server struct {
	reader    *bufio.Reader
	writer    io.Writer
	mu        sync.Mutex
	documents map[string]string
}

func NewServer(in io.Reader, out io.Writer) *Server {
	return &Server{reader: bufio.NewReader(in), writer: out, documents: map[string]string{}}
}

func (s *Server) Serve() error {
	for {
		request, err := s.read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if s.handle(request) {
			return nil
		}
	}
}

func (s *Server) handle(request Request) bool {
	switch request.Method {
	case MethodInitialize:
		s.reply(request, InitializeResult{Capabilities: capabilities()})
	case MethodShutdown:
		s.reply(request, nil)
	case MethodExit:
		return true
	case MethodDidOpen:
		s.opened(request)
	case MethodDidChange:
		s.changed(request)
	case MethodDidClose:
		s.dropped(request)
	case MethodCompletion:
		s.completion(request)
	case MethodHover:
		s.hover(request)
	default:
		if len(request.ID) > 0 {
			s.reply(request, nil)
		}
	}
	return false
}

func capabilities() ServerCapabilities {
	return ServerCapabilities{
		TextDocumentSync:   1,
		HoverProvider:      true,
		CompletionProvider: &CompletionOptions{TriggerCharacters: []string{".", "{", "%", "|"}},
	}
}

func (s *Server) opened(request Request) {
	var params DidOpenParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return
	}
	s.store(params.TextDocument.URI, params.TextDocument.Text)
	s.publish(params.TextDocument.URI)
}

func (s *Server) changed(request Request) {
	var params DidChangeParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return
	}
	if len(params.ContentChanges) == 0 {
		return
	}
	s.store(params.TextDocument.URI, params.ContentChanges[len(params.ContentChanges)-1].Text)
	s.publish(params.TextDocument.URI)
}

func (s *Server) dropped(request Request) {
	var params DidOpenParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return
	}
	s.mu.Lock()
	delete(s.documents, params.TextDocument.URI)
	s.mu.Unlock()
}

func (s *Server) completion(request Request) {
	params, text, ok := s.document(request)
	if !ok {
		s.reply(request, []CompletionItem{})
		return
	}
	s.reply(request, Analyse(nameOf(params.TextDocument.URI), text).Completions(params.Position))
}

func (s *Server) hover(request Request) {
	params, text, ok := s.document(request)
	if !ok {
		s.reply(request, nil)
		return
	}
	hover, found := Analyse(nameOf(params.TextDocument.URI), text).Hover(params.Position)
	if !found {
		s.reply(request, nil)
		return
	}
	s.reply(request, hover)
}

func (s *Server) document(request Request) (DocumentParams, string, bool) {
	var params DocumentParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return params, "", false
	}
	s.mu.Lock()
	text, ok := s.documents[params.TextDocument.URI]
	s.mu.Unlock()
	return params, text, ok
}

func (s *Server) store(uri, text string) {
	s.mu.Lock()
	s.documents[uri] = text
	s.mu.Unlock()
}

func (s *Server) publish(uri string) {
	s.mu.Lock()
	text := s.documents[uri]
	s.mu.Unlock()
	report := Analyse(nameOf(uri), text).Report()
	s.notify(MethodDiagnostics, PublishParams{URI: uri, Diagnostics: report})
}

func nameOf(uri string) string {
	return strings.TrimPrefix(uri, "file://")
}

func (s *Server) reply(request Request, result any) {
	s.send(Response{JSONRPC: "2.0", ID: request.ID, Result: result})
}

func (s *Server) notify(method string, params any) {
	s.send(Response{JSONRPC: "2.0", Method: method, Params: params})
}

func (s *Server) send(response Response) {
	body, err := json.Marshal(response)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = fmt.Fprintf(s.writer, "Content-Length: %d\r\n\r\n%s", len(body), body)
}

func (s *Server) read() (Request, error) {
	length := 0
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			return Request{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		name, value, found := strings.Cut(line, ": ")
		if found && strings.EqualFold(name, "Content-Length") {
			length, err = strconv.Atoi(value)
			if err != nil {
				return Request{}, fmt.Errorf("bad content length %q", value)
			}
		}
	}
	if length <= 0 {
		return Request{}, fmt.Errorf("a message needs a content length")
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(s.reader, body); err != nil {
		return Request{}, err
	}
	var request Request
	if err := json.Unmarshal(body, &request); err != nil {
		return Request{}, err
	}
	return request, nil
}
