package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/nlopes/slack"
	"golang.org/x/oauth2"
	null "gopkg.in/guregu/null.v3"
	gock "gopkg.in/h2non/gock.v1"
)

func createActionCallbackRequest(actionType string, token string) *http.Request {
	callback := &slack.AttachmentActionCallback{
		Actions:     []slack.AttachmentAction{{Name: actionType}},
		Token:       token,
		ResponseURL: "https://hooks.slack.test/coolhook",
		User: slack.User{
			ID: "FOO",
		},
	}
	json, _ := json.Marshal(callback)
	data := url.Values{}
	data.Set("payload", string(json))
	req, _ := http.NewRequest(http.MethodPost, "https://example.com/hooks/interactive", strings.NewReader(data.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Content-Length", strconv.Itoa(len(data.Encode())))
	return req
}

func setupActionCallbackGocks(actionType string, responseText string) {
	if actionType == actionTypeAttend || actionType == actionTypeLeave {
		gock.New("https://teamspirit-1234.cloudforce.test").
			Put("/services/apexrest/Dakoku").
			Reply(200).
			BodyString(responseText)
	} else {
		gock.New("https://teamspirit-1234.cloudforce.test").
			Post("/services/apexrest/Dakoku").
			Reply(200).
			BodyString(responseText)
	}

	gock.New("https://teamspirit-1234.cloudforce.test").
		Get("/services/apexrest/Dakoku").
		Reply(200).
		JSON([]map[string]interface{}{{"from": 1, "to": 2, "type": 1}})
}

func testGetActionCallbackWithActionType(t *testing.T, actionType string, successMessage string) {
	defer gock.Off()
	app := createMockApp()
	app.CleanRedis()
	req, _ := http.NewRequest(http.MethodPost, "https://example.com/hooks/interactive", strings.NewReader(""))
	ctx := app.createContext(req)
	ctx.UserID = "FOO"
	ctx.setAccessToken(&oauth2.Token{
		AccessToken:  "foo",
		RefreshToken: "bar",
		TokenType:    "Bearer",
	})

	gock.New("https://teamspirit-1234.cloudforce.test").
		Get("/services/apexrest/Dakoku").
		Reply(200).
		JSON([]map[string]interface{}{{"message": "Session expired or invalid", "errorCode": "INVALID_SESSION_ID"}})

	gock.InterceptClient(ctx.createTimeTableClient().HTTPClient)

	msg, responseURL, err := ctx.getActionCallback(&slack.AttachmentActionCallback{
		Actions:     []slack.AttachmentAction{{Name: actionType}},
		Token:       app.SlackVerificationToken,
		ResponseURL: "https://hooks.slack.test/coolhook",
		User: slack.User{
			ID: "FOO",
		},
	})

	for _, test := range []Test{
		{true, err == nil},
		{"https://hooks.slack.test/coolhook", responseURL},
		{"TeamSpirit で認証を行って、再度 `/ts` コマンドを実行してください :bow:", msg.Attachments[0].Text},
		{0, strings.Index(msg.Attachments[0].Actions[0].URL, "https://example.com/oauth/authenticate/")},
		{true, gock.IsDone()},
	} {
		test.Compare(t)
	}

	setupActionCallbackGocks(actionType, `"OK"`)
	msg, responseURL, err = ctx.getActionCallback(&slack.AttachmentActionCallback{
		Actions:     []slack.AttachmentAction{{Name: actionType}},
		Token:       app.SlackVerificationToken,
		ResponseURL: "https://hooks.slack.test/coolhook",
		User: slack.User{
			ID: "FOO",
		},
	})

	for _, test := range []Test{
		{true, err == nil},
		{"https://hooks.slack.test/coolhook", responseURL},
		{successMessage, msg.Text},
		{"in_channel", msg.ResponseType},
		{true, msg.ReplaceOriginal},
		{true, gock.IsDone()},
	} {
		test.Compare(t)
	}

	setupActionCallbackGocks(actionType, "NG")

	msg, responseURL, err = ctx.getActionCallback(&slack.AttachmentActionCallback{
		Actions:     []slack.AttachmentAction{{Name: actionType}},
		Token:       app.SlackVerificationToken,
		ResponseURL: "https://hooks.slack.test/coolhook",
		User: slack.User{
			ID: "FOO",
		},
	})

	for _, test := range []Test{
		{true, err == nil},
		{"https://hooks.slack.test/coolhook", responseURL},
		{"勤務表の更新に失敗しました :warning:", msg.Text},
		{"ephemeral", msg.ResponseType},
		{false, msg.ReplaceOriginal},
		{true, gock.IsDone()},
	} {
		test.Compare(t)
	}
}

func TestGetActionCallback(t *testing.T) {
	testGetActionCallbackWithActionType(t, actionTypeAttend, "出社しました :office:")
	testGetActionCallbackWithActionType(t, actionTypeLeave, "退社しました :house:")
	testGetActionCallbackWithActionType(t, actionTypeRest, "休憩を開始しました :coffee:")
	testGetActionCallbackWithActionType(t, actionTypeUnrest, "休憩を終了しました :computer:")
}

func setupTimeTableGocks(items []timeTableItem) {
	gock.New("https://teamspirit-1234.cloudforce.test").
		Get("/services/apexrest/Dakoku").
		Reply(200).
		JSON(items)
}

func TestGetSlackMessage(t *testing.T) {
	defer gock.Off()
	app := createMockApp()
	req, _ := http.NewRequest(http.MethodPost, "https://example.com/hooks/slash", bytes.NewBufferString(""))
	ctx := app.createContext(req)
	ctx.UserID = "BAZ"
	msg, err := ctx.getSlackMessage("")
	for _, test := range []Test{
		{true, err == nil},
		{"TeamSpirit で認証を行って、再度 `/ts` コマンドを実行してください :bow:", msg.Attachments[0].Text},
		{0, strings.Index(msg.Attachments[0].Actions[0].URL, "https://example.com/oauth/authenticate/")},
	} {
		test.Compare(t)
	}
	ctx.setAccessToken(&oauth2.Token{
		AccessToken:  "foo",
		RefreshToken: "bar",
		TokenType:    "Bearer",
	})
	ctx.TimeTableClient = nil
	msg, err = ctx.getSlackMessage("")
	for _, test := range []Test{
		{true, err == nil},
		{"TeamSpirit で認証を行って、再度 `/ts` コマンドを実行してください :bow:", msg.Attachments[0].Text},
		{0, strings.Index(msg.Attachments[0].Actions[0].URL, "https://example.com/oauth/authenticate/")},
	} {
		test.Compare(t)
	}
	setupTimeTableGocks([]timeTableItem{
		{null.IntFrom(10 * 60), null.IntFrom(19 * 60), 1},
	})
	ctx.TimeTableClient = nil
	msg, err = ctx.getSlackMessage("")
	for _, test := range []Test{
		{true, err == nil},
		{"既に退社済です。打刻修正は <https://teamspirit-1234.cloudforce.test|TeamSpirit> で行なってください。", msg.Text},
		{true, gock.IsDone()},
	} {
		test.Compare(t)
	}
	setupTimeTableGocks([]timeTableItem{
		{null.IntFrom(10 * 60), null.IntFromPtr(nil), 1},
		{null.IntFrom(10 * 60), null.IntFromPtr(nil), 21},
	})
	ctx.TimeTableClient = nil
	msg, err = ctx.getSlackMessage("")
	for _, test := range []Test{
		{true, err == nil},
		{"休憩を終了する", msg.Attachments[0].Actions[0].Text},
		{actionTypeUnrest, msg.Attachments[0].Actions[0].Name},
		{true, gock.IsDone()},
	} {
		test.Compare(t)
	}
	setupTimeTableGocks([]timeTableItem{
		{null.IntFrom(10 * 60), null.IntFromPtr(nil), 1},
		{null.IntFrom(10 * 60), null.IntFromPtr(nil), 21},
	})
	ctx.TimeTableClient = nil
	msg, err = ctx.getSlackMessage("")
	for _, test := range []Test{
		{true, err == nil},
		{"休憩を終了する", msg.Attachments[0].Actions[0].Text},
		{actionTypeUnrest, msg.Attachments[0].Actions[0].Name},
		{true, gock.IsDone()},
	} {
		test.Compare(t)
	}
	setupTimeTableGocks([]timeTableItem{
		{null.IntFrom(10 * 60), null.IntFromPtr(nil), 1},
		{null.IntFrom(10 * 60), null.IntFrom(11 * 60), 21},
	})
	ctx.TimeTableClient = nil
	msg, err = ctx.getSlackMessage("")
	for _, test := range []Test{
		{true, err == nil},
		{"休憩を開始する", msg.Attachments[0].Actions[0].Text},
		{actionTypeRest, msg.Attachments[0].Actions[0].Name},
		{"退社する", msg.Attachments[0].Actions[1].Text},
		{actionTypeLeave, msg.Attachments[0].Actions[1].Name},
		{true, gock.IsDone()},
	} {
		test.Compare(t)
	}
	setupTimeTableGocks([]timeTableItem{
		{null.IntFromPtr(nil), null.IntFromPtr(nil), 1},
		{null.IntFrom(10 * 60), null.IntFrom(11 * 60), 21},
	})
	ctx.TimeTableClient = nil
	msg, err = ctx.getSlackMessage("")
	for _, test := range []Test{
		{true, err == nil},
		{"出社する", msg.Attachments[0].Actions[0].Text},
		{actionTypeAttend, msg.Attachments[0].Actions[0].Name},
		{true, gock.IsDone()},
	} {
		test.Compare(t)
	}
}
