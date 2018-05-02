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

package e2e

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/topfreegames/pitaya/client"
	"github.com/topfreegames/pitaya/helpers"
	"github.com/topfreegames/pitaya/internal/message"
)

var update = flag.Bool("update", false, "update server binary")

func TestMain(m *testing.M) {
	flag.Parse()
	if *update {
		cmd := exec.Command("go", "build", "-o", "../examples/testing/server", "../examples/testing/main.go")
		err := cmd.Run()
		if err != nil {
			panic(err)
		}
	}
	exit := m.Run()
	os.Exit(exit)
}

func TestHandlerCallToFront(t *testing.T) {
	tables := []struct {
		req  string
		data []byte
		resp []byte
	}{
		{"connector.testsvc.testrequestonlysessionreturnsptr", []byte(``), []byte(`{"code":200,"msg":"hello"}`)},
		{"connector.testsvc.testrequestonlysessionreturnsptrnil", []byte(``), []byte(`{"Code":"PIT-000","Msg":"reply must not be null"}`)},
		{"connector.testsvc.testrequestonlysessionreturnsrawnil", []byte(``), []byte(`{"Code":"PIT-000","Msg":"reply must not be null"}`)},
		{"connector.testsvc.testrequestreturnsptr", []byte(`{"msg":"good"}`), []byte(`{"code":200,"msg":"good"}`)},
		{"connector.testsvc.testrequestreturnsraw", []byte(`{"msg":"good"}`), []byte(`good`)},
		{"connector.testsvc.testrequestreceivereturnsraw", []byte(`woow`), []byte(`woow`)},
		{"connector.testsvc.nonexistenthandler", []byte(`woow`), []byte(`{"Code":"PIT-404","Msg":"pitaya/handler: connector.testsvc.nonexistenthandler not found"}`)},
		{"connector.testsvc.testrequestreturnserror", []byte(`woow`), []byte(`{"Code":"PIT-555","Msg":"somerror"}`)},
	}
	port := helpers.GetFreePort(t)
	sdPrefix := fmt.Sprintf("%s/", uuid.New().String())

	defer helpers.StartServer(t, true, true, "connector", port, sdPrefix)()
	c := client.New(logrus.InfoLevel)

	err := c.ConnectTo(fmt.Sprintf("localhost:%d", port))
	assert.NoError(t, err)
	defer c.Disconnect()

	for _, table := range tables {
		t.Run(table.req, func(t *testing.T) {
			err = c.SendRequest(table.req, table.data)
			assert.NoError(t, err)

			msg := helpers.ShouldEventuallyReceive(t, c.IncomingMsgChan).(*message.Message)
			assert.Equal(t, message.Response, msg.Type)
			assert.Equal(t, table.resp, msg.Data)
		})
	}
}

func TestGroupFront(t *testing.T) {
	port := helpers.GetFreePort(t)

	sdPrefix := fmt.Sprintf("%s/", uuid.New().String())
	defer helpers.StartServer(t, true, true, "connector", port, sdPrefix)()
	c1 := client.New(logrus.InfoLevel)
	c2 := client.New(logrus.InfoLevel)

	err := c1.ConnectTo(fmt.Sprintf("localhost:%d", port))
	assert.NoError(t, err)
	defer c1.Disconnect()

	err = c2.ConnectTo(fmt.Sprintf("localhost:%d", port))
	assert.NoError(t, err)
	defer c2.Disconnect()

	err = c1.SendRequest("connector.testsvc.testbind", []byte{})
	assert.NoError(t, err)
	err = c2.SendRequest("connector.testsvc.testbind", []byte{})
	assert.NoError(t, err)

	msg1 := helpers.ShouldEventuallyReceive(t, c1.IncomingMsgChan).(*message.Message)
	msg2 := helpers.ShouldEventuallyReceive(t, c2.IncomingMsgChan).(*message.Message)

	assert.Equal(t, []byte("ack"), msg1.Data)
	assert.Equal(t, []byte("ack"), msg2.Data)

	tables := []struct {
		route string
		data  []byte
	}{
		{"connector.testsvc.testsendgroupmsg", []byte("testing group")},
		{"connector.testsvc.testsendgroupmsgptr", []byte(`{"msg":"hellow"}`)},
	}

	for _, table := range tables {
		c1.SendNotify(table.route, table.data)
		msg1 = helpers.ShouldEventuallyReceive(t, c1.IncomingMsgChan).(*message.Message)
		msg2 = helpers.ShouldEventuallyReceive(t, c2.IncomingMsgChan).(*message.Message)

		assert.Equal(t, message.Push, msg1.Type)
		assert.Equal(t, message.Push, msg2.Type)

		assert.Equal(t, table.data, msg1.Data)
		assert.Equal(t, table.data, msg2.Data)
	}
}

func TestKick(t *testing.T) {
	port1 := helpers.GetFreePort(t)

	sdPrefix := fmt.Sprintf("%s/", uuid.New().String())
	defer helpers.StartServer(t, true, true, "connector", port1, sdPrefix)()
	c1 := client.New(logrus.InfoLevel)
	c2 := client.New(logrus.InfoLevel)

	err := c1.ConnectTo(fmt.Sprintf("localhost:%d", port1))
	assert.NoError(t, err)
	defer c1.Disconnect()

	err = c2.ConnectTo(fmt.Sprintf("localhost:%d", port1))
	assert.NoError(t, err)
	defer c2.Disconnect()

	uid1 := uuid.New().String()
	err = c1.SendRequest("connector.testsvc.testbindid", []byte(uid1))
	assert.NoError(t, err)
	err = c2.SendRequest("connector.testsvc.testrequestkickuser", []byte(uid1))
	assert.NoError(t, err)

	helpers.ShouldEventuallyReturn(t, func() bool {
		return c1.Connected
	}, false)
}

func TestKickOnBack(t *testing.T) {
	// TODO do not skip
	t.Skip()
	port1 := helpers.GetFreePort(t)

	sdPrefix := fmt.Sprintf("%s/", uuid.New().String())
	defer helpers.StartServer(t, true, true, "connector", port1, sdPrefix)()
	defer helpers.StartServer(t, false, true, "game", 0, sdPrefix)()
	c1 := client.New(logrus.InfoLevel)
	c2 := client.New(logrus.InfoLevel)

	err := c1.ConnectTo(fmt.Sprintf("localhost:%d", port1))
	assert.NoError(t, err)
	defer c1.Disconnect()

	err = c2.ConnectTo(fmt.Sprintf("localhost:%d", port1))
	assert.NoError(t, err)
	defer c2.Disconnect()

	uid1 := uuid.New().String()
	err = c1.SendRequest("game.testsvc.testbindid", []byte(uid1))
	assert.NoError(t, err)
	err = c1.SendRequest("game.testsvc.testrequestkickme", nil)
	assert.NoError(t, err)

	helpers.ShouldEventuallyReturn(t, func() bool {
		return c1.Connected
	}, false)
}

func TestPushToUsers(t *testing.T) {
	port1 := helpers.GetFreePort(t)

	sdPrefix := fmt.Sprintf("%s/", uuid.New().String())
	defer helpers.StartServer(t, false, true, "game", 0, sdPrefix)()
	defer helpers.StartServer(t, true, true, "connector", port1, sdPrefix)()
	port2 := helpers.GetFreePort(t)
	defer helpers.StartServer(t, true, true, "connector", port2, sdPrefix)()
	c1 := client.New(logrus.InfoLevel)
	c2 := client.New(logrus.InfoLevel)

	err := c1.ConnectTo(fmt.Sprintf("localhost:%d", port1))
	assert.NoError(t, err)
	defer c1.Disconnect()

	err = c2.ConnectTo(fmt.Sprintf("localhost:%d", port2))
	assert.NoError(t, err)
	defer c2.Disconnect()

	uid1 := uuid.New().String()
	err = c1.SendRequest("connector.testsvc.testbindid", []byte(uid1))
	assert.NoError(t, err)
	uid2 := uuid.New().String()
	err = c2.SendRequest("connector.testsvc.testbindid", []byte(uid2))
	assert.NoError(t, err)

	msg1 := helpers.ShouldEventuallyReceive(t, c1.IncomingMsgChan).(*message.Message)
	msg2 := helpers.ShouldEventuallyReceive(t, c2.IncomingMsgChan).(*message.Message)

	assert.Equal(t, []byte("ack"), msg1.Data)
	assert.Equal(t, []byte("ack"), msg2.Data)

	msg := fmt.Sprintf(`{"msg":"testing send to users","uids":["%s","%s"]}`, uid1, uid2)

	tables := []struct {
		route string
	}{
		{"connector.testsvc.testsendtousers"},
		{"game.testsvc.testsendtousers"},
	}

	for _, table := range tables {
		c1.SendNotify(table.route, []byte(msg))
		msg1 = helpers.ShouldEventuallyReceive(t, c1.IncomingMsgChan).(*message.Message)
		msg2 = helpers.ShouldEventuallyReceive(t, c2.IncomingMsgChan).(*message.Message)

		assert.Equal(t, message.Push, msg1.Type)
		assert.Equal(t, message.Push, msg2.Type)

		assert.Equal(t, "testing send to users", string(msg1.Data))
		assert.Equal(t, "testing send to users", string(msg2.Data))
	}
}

func TestForwardToBackend(t *testing.T) {
	portFront := helpers.GetFreePort(t)
	sdPrefix := fmt.Sprintf("%s/", uuid.New().String())
	defer helpers.StartServer(t, false, true, "game", 0, sdPrefix)()
	defer helpers.StartServer(t, true, true, "connector", portFront, sdPrefix)()

	tables := []struct {
		req  string
		data []byte
		resp []byte
	}{
		{"game.testsvc.testrequestonlysessionreturnsptr", []byte(``), []byte(`{"code":200,"msg":"hello"}`)},
		{"game.testsvc.testrequestreturnsptr", []byte(`{"msg":"good"}`), []byte(`{"code":200,"msg":"good"}`)},
		{"game.testsvc.testrequestreturnsraw", []byte(`{"msg":"good"}`), []byte(`good`)},
		{"game.testsvc.testrequestreceivereturnsraw", []byte(`woow`), []byte(`woow`)},
		{"game.testsvc.nonexistenthandler", []byte(`woow`), []byte(`{"Code":"PIT-404","Msg":"pitaya/handler: game.testsvc.nonexistenthandler not found"}`)},
		{"game.testsvc.testrequestreturnserror", []byte(`woow`), []byte(`{"Code":"PIT-555","Msg":"somerror"}`)},
	}

	c := client.New(logrus.InfoLevel)

	err := c.ConnectTo(fmt.Sprintf("localhost:%d", portFront))
	assert.NoError(t, err)
	defer c.Disconnect()

	for _, table := range tables {
		t.Run(table.req, func(t *testing.T) {
			err = c.SendRequest(table.req, table.data)
			assert.NoError(t, err)

			msg := helpers.ShouldEventuallyReceive(t, c.IncomingMsgChan).(*message.Message)
			assert.Equal(t, message.Response, msg.Type)
			assert.Equal(t, table.resp, msg.Data)
		})
	}
}

func TestGroupBack(t *testing.T) {
	portFront1 := helpers.GetFreePort(t)
	portFront2 := helpers.GetFreePort(t)
	sdPrefix := fmt.Sprintf("%s/", uuid.New().String())

	defer helpers.StartServer(t, false, true, "game", 0, sdPrefix)()
	defer helpers.StartServer(t, true, true, "connector", portFront1, sdPrefix)()
	defer helpers.StartServer(t, true, true, "connector", portFront2, sdPrefix)()
	c1 := client.New(logrus.InfoLevel)
	c2 := client.New(logrus.InfoLevel)

	err := c1.ConnectTo(fmt.Sprintf("localhost:%d", portFront1))
	assert.NoError(t, err)
	defer c1.Disconnect()

	err = c2.ConnectTo(fmt.Sprintf("localhost:%d", portFront2))
	assert.NoError(t, err)
	defer c2.Disconnect()

	err = c1.SendRequest("game.testsvc.testbind", []byte{})
	assert.NoError(t, err)
	err = c2.SendRequest("game.testsvc.testbind", []byte{})
	assert.NoError(t, err)

	msg1 := helpers.ShouldEventuallyReceive(t, c1.IncomingMsgChan).(*message.Message)
	msg2 := helpers.ShouldEventuallyReceive(t, c2.IncomingMsgChan).(*message.Message)

	assert.Equal(t, []byte("ack"), msg1.Data)
	assert.Equal(t, []byte("ack"), msg2.Data)

	tables := []struct {
		route string
		data  []byte
	}{
		{"game.testsvc.testsendgroupmsg", []byte("testing group")},
		{"game.testsvc.testsendgroupmsgptr", []byte(`{"msg":"hellow"}`)},
	}

	for _, table := range tables {
		c1.SendNotify(table.route, table.data)
		msg1 = helpers.ShouldEventuallyReceive(t, c1.IncomingMsgChan).(*message.Message)
		msg2 = helpers.ShouldEventuallyReceive(t, c2.IncomingMsgChan).(*message.Message)

		assert.Equal(t, message.Push, msg1.Type)
		assert.Equal(t, message.Push, msg2.Type)

		assert.Equal(t, table.data, msg1.Data)
		assert.Equal(t, table.data, msg2.Data)
	}
}

func TestUserRPC(t *testing.T) {
	portFront1 := helpers.GetFreePort(t)

	sdPrefix := fmt.Sprintf("%s/", uuid.New().String())
	defer helpers.StartServer(t, false, true, "game", 0, sdPrefix)()
	defer helpers.StartServer(t, true, true, "connector", portFront1, sdPrefix)()
	c1 := client.New(logrus.InfoLevel)

	err := c1.ConnectTo(fmt.Sprintf("localhost:%d", portFront1))
	assert.NoError(t, err)
	defer c1.Disconnect()

	tables := []struct {
		name  string
		route string
		data  []byte
		res   []byte
	}{
		{"front_to_back", "connector.testsvc.testsendrpc", []byte(`{"route":"game.testremotesvc.rpctestrawptrreturnsptr","data":"thisthis"}`), []byte(`{"code":200,"msg":"got thisthis"}`)},
		{"back_to_front", "game.testsvc.testsendrpc", []byte(`{"route":"connector.testremotesvc.rpctestrawptrreturnsptr","data":"thisthis"}`), []byte(`{"code":200,"msg":"got thisthis"}`)},
		{"front_to_back_error", "connector.testsvc.testsendrpc", []byte(`{"route":"game.testremotesvc.rpctestreturnserror","data":"thisthis"}`), []byte(`{"Code":"PIT-433","Msg":"test error","Metadata":{"some":"meta"}}`)},
		{"back_to_front_error", "game.testsvc.testsendrpc", []byte(`{"route":"connector.testremotesvc.rpctestreturnserror","data":"thisthis"}`), []byte(`{"Code":"PIT-433","Msg":"test error","Metadata":{"some":"meta"}}`)},
		{"same_server", "connector.testsvc.testsendrpc", []byte(`{"route":"connector.testremotesvc.rpctestrawptrreturnsptr","data":"thisthis"}`), []byte(`{"Code":"PIT-000","Msg":"you are making a rpc that may be processed locally, either specify a different server type or specify a server id"}`)},
		{"front_to_back_ptr", "connector.testsvc.testsendrpcpointer", []byte(`{"route":"game.testremotesvc.rpctestptrreturnsptr","data":"thisthis"}`), []byte(`{"code":200,"msg":"got thisthis"}`)},
		{"no_args", "connector.testsvc.testsendrpcnoargs", []byte(`{"route":"game.testremotesvc.rpctestnoargs"}`), []byte(`{"code":200,"msg":"got nothing"}`)},
		{"not_found", "connector.testsvc.testsendrpcpointer", []byte(`{"route":"game.testremotesvc.rpctestnotfound","data":"thisthis"}`), []byte(`{"Code":"PIT-404","Msg":"route not found","Metadata":{"route":"testremotesvc.rpctestnotfound"}}`)},
	}

	for _, table := range tables {
		t.Run(table.name, func(t *testing.T) {
			err := c1.SendRequest(table.route, table.data)
			assert.NoError(t, err)
			msg := helpers.ShouldEventuallyReceive(t, c1.IncomingMsgChan).(*message.Message)
			assert.Equal(t, table.res, msg.Data)
		})
	}
}
