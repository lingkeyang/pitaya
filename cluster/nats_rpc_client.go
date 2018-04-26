// Copyright (c) TFG Co. All Rights Reserved.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package cluster

import (
	"context"
	"fmt"
	"time"

	"github.com/gogo/protobuf/proto"
	nats "github.com/nats-io/go-nats"
	"github.com/topfreegames/pitaya/config"
	"github.com/topfreegames/pitaya/constants"
	pcontext "github.com/topfreegames/pitaya/context"
	"github.com/topfreegames/pitaya/errors"
	"github.com/topfreegames/pitaya/internal/message"
	"github.com/topfreegames/pitaya/protos"
	"github.com/topfreegames/pitaya/route"
	"github.com/topfreegames/pitaya/session"
	"github.com/topfreegames/pitaya/util"
)

// NatsRPCClient struct
type NatsRPCClient struct {
	config     *config.Config
	conn       *nats.Conn
	connString string
	reqTimeout time.Duration
	running    bool
	server     *Server
}

// NewNatsRPCClient ctor
func NewNatsRPCClient(config *config.Config, server *Server) (*NatsRPCClient, error) {
	ns := &NatsRPCClient{
		config:  config,
		server:  server,
		running: false,
	}
	if err := ns.configure(); err != nil {
		return nil, err
	}
	return ns, nil
}

func (ns *NatsRPCClient) configure() error {
	ns.connString = ns.config.GetString("pitaya.cluster.rpc.client.nats.connect")
	if ns.connString == "" {
		return constants.ErrNoNatsConnectionString
	}
	ns.reqTimeout = ns.config.GetDuration("pitaya.cluster.rpc.client.nats.requesttimeout")
	if ns.reqTimeout == 0 {
		return constants.ErrNatsNoRequestTimeout
	}
	return nil
}

// Send publishes a message in a given topic
func (ns *NatsRPCClient) Send(topic string, data []byte) error {
	if !ns.running {
		return constants.ErrRPCClientNotInitialized
	}
	return ns.conn.Publish(topic, data)
}

func (ns *NatsRPCClient) buildRequest(
	ctx context.Context,
	rpcType protos.RPCType,
	route *route.Route,
	session *session.Session,
	msg *message.Message,
) protos.Request {
	req := protos.Request{
		Type: rpcType,
		Msg: &protos.Msg{
			Route: route.String(),
			Data:  msg.Data,
		},
	}
	m := pcontext.ToMap(ctx)
	if len(m) > 0 {
		b, err := util.GobEncode(m)
		if err != nil {
			// TODO: handle err
		}
		req.Metadata = b
	}
	if ns.server.Frontend {
		req.FrontendID = ns.server.ID
	}

	switch msg.Type {
	case message.Request:
		req.Msg.Type = protos.MsgType_MsgRequest
	case message.Notify:
		req.Msg.Type = protos.MsgType_MsgNotify
	}

	if rpcType == protos.RPCType_Sys {
		mid := uint(0)
		if msg.Type == message.Request {
			mid = msg.ID
		}
		req.Msg.ID = uint64(mid)
		req.Session = &protos.Session{
			ID:   session.ID(),
			Uid:  session.UID(),
			Data: session.GetDataEncoded(),
		}
	}

	return req
}

// Call calls a method remotally
func (ns *NatsRPCClient) Call(
	ctx context.Context,
	rpcType protos.RPCType,
	route *route.Route,
	session *session.Session,
	msg *message.Message,
	server *Server,
) (*protos.Response, error) {
	if !ns.running {
		return nil, constants.ErrRPCClientNotInitialized
	}
	req := ns.buildRequest(ctx, rpcType, route, session, msg)
	marshalledData, err := proto.Marshal(&req)
	if err != nil {
		return nil, err
	}
	m, err := ns.conn.Request(getChannel(server.Type, server.ID), marshalledData, ns.reqTimeout)
	if err != nil {
		return nil, err
	}

	res := &protos.Response{}
	err = proto.Unmarshal(m.Data, res)

	if err != nil {
		return nil, err
	}

	if res.Error != nil {
		if res.Error.Code == "" {
			res.Error.Code = errors.ErrUnknownCode
		}
		err := &errors.Error{
			Code:     res.Error.Code,
			Message:  res.Error.Msg,
			Metadata: res.Error.Metadata,
		}
		return nil, err
	}
	return res, nil
}

// Init inits nats rpc server
func (ns *NatsRPCClient) Init() error {
	ns.running = true
	conn, err := setupNatsConn(ns.connString)
	if err != nil {
		return err
	}
	ns.conn = conn
	return nil
}

// AfterInit runs after initialization
func (ns *NatsRPCClient) AfterInit() {}

// BeforeShutdown runs before shutdown
func (ns *NatsRPCClient) BeforeShutdown() {}

// Shutdown stops nats rpc server
func (ns *NatsRPCClient) Shutdown() error {
	return nil
}

func (ns *NatsRPCClient) stop() {
	ns.running = false
}

func (ns *NatsRPCClient) getSubscribeChannel() string {
	return fmt.Sprintf("pitaya/servers/%s/%s", ns.server.Type, ns.server.ID)
}
