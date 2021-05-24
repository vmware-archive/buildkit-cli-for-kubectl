package proxy

import (
	"context"
	"fmt"

	control "github.com/moby/buildkit/api/services/control"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth"
	"github.com/moby/buildkit/session/filesync"
	"github.com/moby/buildkit/session/grpchijack"
	"github.com/moby/buildkit/session/secrets"
	"github.com/moby/buildkit/session/sshforward"
	"github.com/moby/buildkit/session/upload"
	"github.com/opentracing/opentracing-go"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/metadata"
)

/*
BuildKits gRPC API uses an interesting pattern for bidirectional communication between the client
and buildkitd.  The initial build request goes from client to server, however, the client does not
send "everything" required for the build upfront, since the client doesn't necessarily know exactly
what the daemon will need to perform the build.  As part of the API, the client and daemon establish
a "reversed" connection from buildkitd to the client, where the client functions as an API server.
This allows buildkitd to send requests to the client to retrieve (or send) content as needed during
the build.  This is called a "Session" and uses a hijacking pattern to overlay a "nested" gRPC
connection on top of an underlying gRPC byte stream API.

The session handling flow looks like the following

	buildkitd <session.Manager> --grpc--> [ this proxy implementation ] --grpc--> <session.Session> CLI

The session.Manager is the API "client" side of the flow
The session.Session is the API "server" side of the flow

session.Manager helps multiplex multiple NewXXXClient implementations, which are emitted gRPC clients.
session.Session creates a grpc.Server and supports "Attachable" grpc Servers for demultiplexing
These grpc Servers implement the "real" logic in the CLI to satisfy the session requests,
such as pulling files from the build context, retrieving credentials, sending the final build results, etc.

The proxy implementation looks like this:

grpc--> <session.Session> [custom server proxy impls] --> [grpc Clients] <session.Manager> --grpc-->

The custom server impls are mostly "pass through" to the corresponding grpc Clients for
those protobuf definitions.  Ex. authproxy.go, filesyncproxy.go, etc.
Any logic we want to hijack will perform the alternate logic here
The grpc Clients are unmodified and wired up to the session.Manager hooked up the grpc
connection to the CLI.

Over time, BuildKit may modify or add APIs on this Session construct.  When/if this
happens this implementation will need to be updated to proxy those new APIs.  If
this becomes a maintenance headache, we may want to explore if there's an
alternative lower-level way to wire this up to only hijack specific grpc server
definitions, and for the rest, do "raw" pass through so we can be protobuf wire
agnostic.  Time will tell, but hopefully vendor bumps will largely be sufficient.
*/

func (s *server) Session(clientStream control.Control_SessionServer) error {
	// Note: buildkit's protocol relies heavily on metadata embedded in the gRPC headers
	//       exposed at the context level.
	ctx := clientStream.Context()
	md, _ := metadata.FromIncomingContext(ctx)
	ctx = metadata.NewOutgoingContext(ctx, md)

	// TODO - This opentracing wiring is probably not right.
	//        Do some experimentation to get the opentracing flowing through the proxy correctly
	statusContext, cancelStatus := context.WithCancel(ctx)
	defer cancelStatus()
	if span := opentracing.SpanFromContext(ctx); span != nil {
		statusContext = opentracing.ContextWithSpan(statusContext, span)
	}

	// Extract specific fields from the metadata that we need to wire up the Session and Manager
	sessionNameList := md.Get(headerSessionName)
	if len(sessionNameList) != 1 {
		return fmt.Errorf("malformed session name header - %v", sessionNameList)
	}
	sessionName := sessionNameList[0]
	sessionKeyList := md.Get(headerSessionSharedKey)
	if len(sessionKeyList) != 1 {
		return fmt.Errorf("malformed session key header - %v", sessionKeyList)
	}
	sessionKey := sessionKeyList[0]
	sessionIDList := md.Get(headerSessionID)
	if len(sessionIDList) != 1 {
		return fmt.Errorf("malformed session ID header - %v", sessionIDList)
	}
	sessionID := sessionIDList[0]
	logrus.Debugf("starting proxying Session Name=%s ID=%s Key=%s", sessionName, sessionID, sessionKey)
	sessionProxies := s.getSession(sessionID)

	buildkitStream, err := s.buildkitd.Session(ctx) // TODO - is the opentracing context right here?
	if err != nil {
		logrus.Errorf("unable to establish Session stream: %s", err)
		return err
	}
	// Note: This Session object has a bogus auto-generated ID, but we never use it
	//
	//       At present, this routine is structured as a singleton and doesn't
	//       attempt to perform any multiplexing of multiple sessions.
	//       This should work fine since we'll get a discrete call to this routine
	//       for each session.  This makes the session ID tracking unnecessary, but
	//       it's wired up to keep the Manager and Session implementations happy.

	g, ctx := errgroup.WithContext(ctx)

	// Responses sent along to the CLI are sent through a session.Manager
	outgoingManager, err := session.NewManager()
	if err != nil {
		logrus.Errorf("unable to establish outgoing session.Manager: %s", err)
		return err
	}

	conn, closeCh, opts := grpchijack.Hijack(clientStream)
	defer conn.Close()
	ctx2, cancel := context.WithCancel(ctx) // TODO - this cancelable might need to bubble up higher in the ctx stack
	go func() {
		<-closeCh
		cancel()
	}()

	g.Go(func() error {
		err := outgoingManager.HandleConn(ctx2, conn, opts) // TODO - is the opentracing context right here?
		logrus.Debugf("outgoing Manager session finished: %v", err)
		return err
	})

	caller, err := outgoingManager.Get(ctx, sessionID, false)
	if err != nil {
		// This should never happen since we're controlling the IDs...
		logrus.Errorf("session ID miswired in outgoing manager: %s", err)
		return err
	}

	// Now that we're connected, we can create the various grpc clients
	fsyncClient := filesync.NewFileSyncClient(caller.Conn())
	fsendClient := filesync.NewFileSendClient(caller.Conn())
	authClient := auth.NewAuthClient(caller.Conn())
	secretsClient := secrets.NewSecretsClient(caller.Conn())
	sshClient := sshforward.NewSSHClient(caller.Conn())
	uploadClient := upload.NewUploadClient(caller.Conn())
	// Note: Over time if buildkitd adds new Session features, this list will need to grow...

	// Incoming requests from buildkitd are received by a session.Session
	incomingSession, err := session.NewSession(ctx, sessionName, sessionKey) // TODO - is the opentracing context right here?
	if err != nil {
		logrus.Errorf("unable to establish incoming session.Session from buildkitd: %s", err)
		return err
	}

	// wire up all the custom Attachable proxy implementations...
	fsendp := &fileSendProxy{
		c:         fsendClient,
		sessionID: sessionID,
		loader:    s,
	}
	sessionProxies.fileSendProxy = fsendp
	incomingSession.Allow(fsendp)
	fsyncp := &fileSyncProxy{fsyncClient}
	incomingSession.Allow(fsyncp)
	authProxy := &authProxy{authClient}
	incomingSession.Allow(authProxy)
	secretsProxy := &secretsProxy{secretsClient}
	incomingSession.Allow(secretsProxy)
	sshProxy := &sshProxy{sshClient}
	incomingSession.Allow(sshProxy)
	uploadProxy := &uploadProxy{uploadClient}
	incomingSession.Allow(uploadProxy)
	// Note: Over time if buildkitd adds new Session features, this list will need to grow...

	sw := &sessionWrapper{sc: buildkitStream}
	g.Go(func() error {
		err := incomingSession.Run(statusContext, grpchijack.Dialer(sw)) // TODO opentracing and/or cancelable ctx?
		logrus.Debugf("incoming buildkit Session.Run exited with %v", err)
		return err
	})

	err = g.Wait()
	logrus.Debugf("finished proxying Session Name=%s ID=%s Key=%s Err=%v", sessionName, sessionID, sessionKey, err)
	s.sessionsLock.Lock()
	defer s.sessionsLock.Unlock()
	delete(s.sessions, sessionID)
	return err
}
