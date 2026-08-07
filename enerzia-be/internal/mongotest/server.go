// Package mongotest provides an in-process fake MongoDB server for tests.
//
// The v2 driver keeps its own mock (mtest) in an internal package, so it
// cannot be imported here. This speaks just enough of the MongoDB wire
// protocol — the handshake plus single-document commands — for a real
// mongo.Client to connect, ping and issue commands against it. Tests then
// cover the success paths of the connection code without a live cluster, and
// can make any command fail on demand.
//
// It is not a database: it stores nothing and understands no query language.
// Behaviour beyond the handshake is supplied per command by the test.
package mongotest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/x/bsonx/bsoncore"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/wiremessage"
)

// maxWireVersion advertised in the handshake. 21 corresponds to MongoDB 8.0,
// which is comfortably above the driver's minimum.
const maxWireVersion = 21

// headerLen is the fixed size of a wire-protocol message header.
const headerLen = 16

// CommandResult is the reply the server produces for one command.
type CommandResult struct {
	// Doc is the reply document. When nil, a plain {ok: 1} is sent.
	Doc bson.D
	// Err, when non-empty, makes the command fail with this message instead.
	Err string
	// Code is the server error code paired with Err. Defaults to 1.
	Code int32
}

// Fail builds a result that makes a command return a server error.
func Fail(message string, code int32) CommandResult {
	return CommandResult{Err: message, Code: code}
}

// Reply builds a result returning doc verbatim.
func Reply(doc bson.D) CommandResult {
	return CommandResult{Doc: doc}
}

// Cursor builds a single-batch cursor reply, the shape find and aggregate
// return. Cursor id 0 tells the driver the batch is complete, so no getMore
// follows.
func Cursor(namespace string, docs ...bson.D) CommandResult {
	batch := make(bson.A, 0, len(docs))
	for _, d := range docs {
		batch = append(batch, d)
	}
	return CommandResult{Doc: bson.D{
		{Key: "cursor", Value: bson.D{
			{Key: "id", Value: int64(0)},
			{Key: "ns", Value: namespace},
			{Key: "firstBatch", Value: batch},
		}},
		{Key: "ok", Value: 1},
	}}
}

// Request is one command the server received, with the document that carried
// it. Tests assert on the document to check the query a repository built.
type Request struct {
	Command string
	// Doc is the command body (OP_MSG section 0).
	Doc bson.Raw
	// Sequences holds the OP_MSG document sequences (section 1), keyed by
	// identifier. The driver moves bulk payloads out of the body and into
	// these — writes arrive under "documents", "updates" or "deletes", so an
	// assertion about what was written looks here, not in Doc.
	Sequences map[string][]bson.Raw
}

// Sequence returns the named document sequence, or nil if the command carried
// none.
func (r Request) Sequence(identifier string) []bson.Raw { return r.Sequences[identifier] }

// Server is a fake MongoDB listening on a loopback port.
type Server struct {
	listener net.Listener
	wg       sync.WaitGroup

	mu        sync.Mutex
	responses map[string]CommandResult
	// queued holds per-call replies for a command; the last entry repeats
	// once the queue is exhausted.
	queued map[string][]CommandResult
	seen   []Request
	closed bool
}

// Start launches a fake server and registers its shutdown with tb. The
// returned server is ready to accept connections.
func Start(tb testing.TB) *Server {
	tb.Helper()

	var lc net.ListenConfig
	listener, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		tb.Fatalf("mongotest: listen: %v", err)
	}

	s := &Server{
		listener:  listener,
		responses: make(map[string]CommandResult),
		queued:    make(map[string][]CommandResult),
	}
	s.wg.Add(1)
	go s.acceptLoop()

	tb.Cleanup(s.Close)
	return s
}

// URI returns a direct-connection string pointing at this server. It is a
// standalone, so directConnection skips topology discovery.
func (s *Server) URI() string {
	return fmt.Sprintf("mongodb://%s/?directConnection=true", s.listener.Addr().String())
}

// Respond registers the reply for a command, replacing any previous one. Pair
// it with Fail, Reply or Cursor.
func (s *Server) Respond(command string, result CommandResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.responses[command] = result
}

// RespondSequence registers replies consumed one per call, in order. The last
// entry repeats once the queue runs dry.
//
// Repositories that issue several queries of the same kind — fetch the
// products, then fetch their variants — need different answers to consecutive
// `find` commands, which a single Respond cannot express.
func (s *Server) RespondSequence(command string, results ...CommandResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queued[command] = append([]CommandResult(nil), results...)
}

// ClearCommand removes an override previously set by Respond, restoring the
// default reply.
func (s *Server) ClearCommand(command string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.responses, command)
	delete(s.queued, command)
}

// Requests returns every command received so far, in order, with the document
// that carried it.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.seen...)
}

// LastRequest returns the most recent occurrence of a command. Use it to
// assert the query a repository actually built.
func (s *Server) LastRequest(command string) (Request, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.seen) - 1; i >= 0; i-- {
		if s.seen[i].Command == command {
			return s.seen[i], true
		}
	}
	return Request{}, false
}

// Commands returns the command names received so far, in order.
func (s *Server) Commands() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	names := make([]string, len(s.seen))
	for i, r := range s.seen {
		names[i] = r.Command
	}
	return names
}

// Close stops the listener and waits for in-flight connections to finish. It
// is idempotent, so an explicit call plus the tb.Cleanup call is safe.
func (s *Server) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()

	_ = s.listener.Close()
	s.wg.Wait()
}

// nextResponseLocked picks the reply for a command. A queued sequence wins
// over a fixed Respond; the final queued entry repeats. Caller holds s.mu.
func (s *Server) nextResponseLocked(command string) (CommandResult, bool) {
	if queue, ok := s.queued[command]; ok && len(queue) > 0 {
		result := queue[0]
		if len(queue) > 1 {
			s.queued[command] = queue[1:]
		}
		return result, true
	}
	result, ok := s.responses[command]
	return result, ok
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return // listener closed
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConn(conn)
		}()
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	for {
		req, err := readRequest(conn)
		if err != nil {
			return // client hung up, or sent something unsupported
		}

		s.mu.Lock()
		s.seen = append(s.seen, Request{
			Command:   req.command,
			Doc:       req.doc,
			Sequences: req.sequences,
		})
		override, overridden := s.nextResponseLocked(req.command)
		s.mu.Unlock()

		doc, err := s.replyDoc(req.command, override, overridden)
		if err != nil {
			return
		}

		var out []byte
		if req.opCode == wiremessage.OpQuery {
			out = appendOpReply(req.requestID, doc)
		} else {
			out = appendOpMsg(req.requestID, doc)
		}
		if _, err := conn.Write(out); err != nil {
			return
		}
	}
}

// replyDoc builds the BSON reply for one command.
func (s *Server) replyDoc(command string, override CommandResult, overridden bool) ([]byte, error) {
	var reply bson.D

	switch {
	case overridden && override.Err != "":
		code := override.Code
		if code == 0 {
			code = 1
		}
		reply = bson.D{
			{Key: "ok", Value: 0},
			{Key: "errmsg", Value: override.Err},
			{Key: "code", Value: code},
			{Key: "codeName", Value: "Location" + fmt.Sprint(code)},
		}
	case overridden && override.Doc != nil:
		reply = override.Doc
	case isHandshake(command):
		reply = handshakeDoc()
	default:
		reply = bson.D{{Key: "ok", Value: 1}}
	}

	out, err := bson.Marshal(reply)
	if err != nil {
		return nil, fmt.Errorf("mongotest: marshal reply: %w", err)
	}
	return out, nil
}

func isHandshake(command string) bool {
	switch command {
	case "hello", "isMaster", "ismaster":
		return true
	default:
		return false
	}
}

// handshakeDoc describes a standalone server. The absence of setName and of
// msg:"isdbgrid" is what makes the driver treat it as Standalone rather than a
// replica-set member or mongos.
func handshakeDoc() bson.D {
	return bson.D{
		{Key: "ok", Value: 1},
		{Key: "helloOk", Value: true},
		{Key: "ismaster", Value: true},
		{Key: "isWritablePrimary", Value: true},
		{Key: "minWireVersion", Value: int32(0)},
		{Key: "maxWireVersion", Value: int32(maxWireVersion)},
		{Key: "maxBsonObjectSize", Value: int32(16 * 1024 * 1024)},
		{Key: "maxMessageSizeBytes", Value: int32(48000000)},
		{Key: "maxWriteBatchSize", Value: int32(100000)},
		{Key: "logicalSessionTimeoutMinutes", Value: int32(30)},
		{Key: "connectionId", Value: int32(1)},
		{Key: "localTime", Value: time.Now()},
		{Key: "readOnly", Value: false},
	}
}

// request is one decoded client message.
type request struct {
	requestID int32
	opCode    wiremessage.OpCode
	command   string
	doc       bson.Raw
	sequences map[string][]bson.Raw
}

var errMalformed = errors.New("mongotest: malformed wire message")

func readRequest(conn net.Conn) (request, error) {
	head := make([]byte, headerLen)
	if _, err := io.ReadFull(conn, head); err != nil {
		return request{}, err
	}

	length, requestID, _, opCode, _, ok := wiremessage.ReadHeader(head)
	if !ok || length < headerLen {
		return request{}, errMalformed
	}

	body := make([]byte, length-headerLen)
	if _, err := io.ReadFull(conn, body); err != nil {
		return request{}, err
	}

	doc, sequences, err := extractCommandDoc(opCode, body)
	if err != nil {
		return request{}, err
	}

	command, err := firstKey(doc)
	if err != nil {
		return request{}, err
	}
	// body is freshly allocated per message, so retaining a slice of it is
	// safe — the document is not overwritten by the next read.
	return request{
		requestID: requestID,
		opCode:    opCode,
		command:   command,
		doc:       bson.Raw(doc),
		sequences: sequences,
	}, nil
}

// extractCommandDoc pulls the command body — and any document sequences — out
// of an OP_MSG, or out of the legacy OP_QUERY the driver still uses for the
// very first handshake.
func extractCommandDoc(opCode wiremessage.OpCode, body []byte) (bsoncore.Document, map[string][]bson.Raw, error) {
	switch opCode {
	case wiremessage.OpMsg:
		return readOpMsgSections(body)

	case wiremessage.OpQuery:
		_, rem, ok := wiremessage.ReadQueryFlags(body)
		if !ok {
			return nil, nil, errMalformed
		}
		_, rem, ok = wiremessage.ReadQueryFullCollectionName(rem)
		if !ok {
			return nil, nil, errMalformed
		}
		_, rem, ok = wiremessage.ReadQueryNumberToSkip(rem)
		if !ok {
			return nil, nil, errMalformed
		}
		_, rem, ok = wiremessage.ReadQueryNumberToReturn(rem)
		if !ok {
			return nil, nil, errMalformed
		}
		doc, _, ok := wiremessage.ReadQueryQuery(rem)
		if !ok {
			return nil, nil, errMalformed
		}
		return doc, nil, nil

	default:
		return nil, nil, fmt.Errorf("mongotest: unsupported opcode %v", opCode)
	}
}

// readOpMsgSections walks every section of an OP_MSG. Section 0 is the command
// body; any section 1 is a document sequence carrying a bulk payload the
// driver moved out of the body.
func readOpMsgSections(body []byte) (bsoncore.Document, map[string][]bson.Raw, error) {
	flags, rem, ok := wiremessage.ReadMsgFlags(body)
	if !ok {
		return nil, nil, errMalformed
	}
	// A trailing checksum is not a section; drop it before walking.
	if flags&wiremessage.ChecksumPresent != 0 && len(rem) >= checksumLen {
		rem = rem[:len(rem)-checksumLen]
	}

	var doc bsoncore.Document
	var sequences map[string][]bson.Raw

	for len(rem) > 0 {
		var stype wiremessage.SectionType
		stype, rem, ok = wiremessage.ReadMsgSectionType(rem)
		if !ok {
			return nil, nil, errMalformed
		}

		switch stype {
		case wiremessage.SingleDocument:
			if doc, rem, ok = wiremessage.ReadMsgSectionSingleDocument(rem); !ok {
				return nil, nil, errMalformed
			}
		case wiremessage.DocumentSequence:
			var identifier string
			var data []byte
			if identifier, data, rem, ok = wiremessage.ReadMsgSectionRawDocumentSequence(rem); !ok {
				return nil, nil, errMalformed
			}
			docs, err := splitDocuments(data)
			if err != nil {
				return nil, nil, err
			}
			if sequences == nil {
				sequences = make(map[string][]bson.Raw, 1)
			}
			sequences[identifier] = append(sequences[identifier], docs...)
		default:
			return nil, nil, fmt.Errorf("mongotest: unsupported section type %v", stype)
		}
	}

	if doc == nil {
		return nil, nil, errMalformed
	}
	return doc, sequences, nil
}

// checksumLen is the size of the optional OP_MSG trailing CRC-32C.
const checksumLen = 4

// splitDocuments walks a concatenated run of BSON documents, each prefixed by
// its own little-endian int32 length.
func splitDocuments(data []byte) ([]bson.Raw, error) {
	var out []bson.Raw
	for len(data) > 0 {
		doc, rem, ok := bsoncore.ReadDocument(data)
		if !ok {
			return nil, errMalformed
		}
		out = append(out, bson.Raw(doc))
		data = rem
	}
	return out, nil
}

// firstKey returns the command name, which by protocol is the first field of
// the command document.
func firstKey(doc bsoncore.Document) (string, error) {
	elems, err := doc.Elements()
	if err != nil {
		return "", fmt.Errorf("mongotest: read command document: %w", err)
	}
	if len(elems) == 0 {
		return "", errMalformed
	}
	return elems[0].Key(), nil
}

func appendOpMsg(responseTo int32, doc []byte) []byte {
	idx, dst := wiremessage.AppendHeaderStart(nil, wiremessage.NextRequestID(), responseTo, wiremessage.OpMsg)
	dst = wiremessage.AppendMsgFlags(dst, 0)
	dst = wiremessage.AppendMsgSectionType(dst, wiremessage.SingleDocument)
	dst = append(dst, doc...)
	return bsoncore.UpdateLength(dst, idx, int32(len(dst[idx:])))
}

func appendOpReply(responseTo int32, doc []byte) []byte {
	idx, dst := wiremessage.AppendHeaderStart(nil, wiremessage.NextRequestID(), responseTo, wiremessage.OpReply)
	dst = wiremessage.AppendReplyFlags(dst, wiremessage.AwaitCapable)
	dst = wiremessage.AppendReplyCursorID(dst, 0)
	dst = wiremessage.AppendReplyStartingFrom(dst, 0)
	dst = wiremessage.AppendReplyNumberReturned(dst, 1)
	dst = append(dst, doc...)
	return bsoncore.UpdateLength(dst, idx, int32(len(dst[idx:])))
}
