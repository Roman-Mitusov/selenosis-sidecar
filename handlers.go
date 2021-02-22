package sidecar

import (
	"encoding/json"
	"errors"
	"fmt"
	httpreverseproxy "github.com/Roman-Mitusov/middleware/proxy/http"
	"github.com/Roman-Mitusov/selenosis-sidecar/utils"
	"github.com/gofiber/fiber/v2"
	"net"
	"net/url"
	"path"
	"strings"
	"time"
)

var (
	ports = struct {
		Devtools, Fileserver, Clipboard string
	}{
		Devtools:   "7070",
		Fileserver: "8080",
		Clipboard:  "9090",
	}
)

// Handle WebDriver session create request
func (app *App) HandleSessionCreate(ctx *fiber.Ctx) error {

	done := make(chan func())

	go func() {
		(<-done)()
	}()

	cancel := func() {}

	defer func() {
		done <- cancel
	}()

	cancelFunc := func() {
		_ = utils.DeleteBrowserPod(app.hostname, app.namespace, app.client)
	}

	return (&httpreverseproxy.ReverseProxy{
		PrepareRequest: func(ctx *fiber.Ctx) error {
			ctx.Path(app.proxyPath)
			ctx.Request().URI().SetScheme("http")
			ctx.Request().URI().SetHost(net.JoinHostPort(app.hostname, app.browserPort))

			go func() {
				<-ctx.Context().Done()

				cancel = cancelFunc
			}()
			return nil
		},
		HandleError: func(ctx *fiber.Ctx) error {
			ctx.Status(fiber.StatusServiceUnavailable)
			return nil
		},
		ModifyResponse: func(ctx *fiber.Ctx) error {
			resp := ctx.Response()
			body := resp.Body()
			var err error
			if body != nil {
				var respBody map[string]interface{}
				if err = json.Unmarshal(body, &respBody); err == nil {
					sessionId, ok := respBody["sessionId"].(string)
					if !ok {
						value, ok := respBody["value"]
						if !ok {
							cancel = cancelFunc
							app.logger.Errorf("Unable to extract sessionId from response")
							return errors.New("selenium protocol")
						}
						valueMap, ok := value.(map[string]interface{})
						if !ok {
							cancel = cancelFunc
							app.logger.Errorf("Unable to extract sessionId from response")
							return errors.New("selenium protocol")
						}
						sessionId, ok = valueMap["sessionId"].(string)
						if !ok {
							cancel = cancelFunc
							app.logger.Errorf("unable to extract sessionId from response")
							return errors.New("selenium protocol")
						}
						respBody["value"].(map[string]interface{})["sessionId"] = app.hostname
					} else {
						respBody["sessionId"] = app.hostname
					}

					changedRespBody, _ := json.Marshal(respBody)
					resp.SetBody(changedRespBody)
					resp.Header.SetContentLength(len(changedRespBody))

					sess := &session{
						URL: &url.URL{
							Scheme: "http",
							Host:   net.JoinHostPort(app.hostname, app.browserPort),
							Path:   path.Join(app.proxyPath, sessionId),
						},
						ID: sessionId,
						OnTimeout: onTimeout(app.idleTimeout, func() {
							app.logger.Warn("Session timed out: %s, after %.2fs", sessionId, app.idleTimeout.Seconds())
							cancelFunc()
						}),
						CancelFunc: cancelFunc,
					}
					app.bucket.put(app.hostname, sess)
					app.logger.Infof("New session request completed: %s", sessionId)
					return nil
				}
				cancel = cancelFunc
				app.logger.Errorf("unable to parse response body: %v", err)
				return errors.New("response body parse error")
			}
			cancel = cancelFunc
			app.logger.Errorf("unable to read response body: %v", err)
			return errors.New("response body read error")
		},
	}).Proxy(ctx)

}

// Handle WebDriver requests such as interaction with elements, viewport etc
func (app *App) HandleWebDriverRequests(ctx *fiber.Ctx) error {
	done := make(chan func())

	go func() {
		(<-done)()
	}()

	cancel := func() {}

	defer func() {
		done <- cancel
	}()

	fragments := strings.Split(ctx.Path(), "/")
	id := ctx.Params("sessionId")

	if id == "" {
		return respondWithWebDriverError("WebDriver session is not found", "Unable to process request because sessionId is not found", fiber.StatusNotFound, ctx)
	}

	_, ok := app.bucket.get(id)

	if ok {
		return (&httpreverseproxy.ReverseProxy{
			PrepareRequest: func(c *fiber.Ctx) error {
				r := c.Request()
				c.Request().URI().SetScheme("http")
				sess, ok := app.bucket.get(id)
				if ok {
					app.bucket.Lock()
					defer app.bucket.Unlock()
					select {
					case <-sess.OnTimeout:
						app.logger.Warnf("Session %s timed out", id)
					default:
						close(sess.OnTimeout)
					}

					if c.Method() == fiber.MethodDelete && len(fragments) == 5 {
						cancel = sess.CancelFunc
						app.logger.Infof("session %s delete request", id)
					} else {
						sess.OnTimeout = onTimeout(app.idleTimeout, func() {
							app.logger.Warnf("session timed out: %s, after %.2fs", id, app.idleTimeout.Seconds())
							err := utils.DeleteBrowserPod(app.hostname, app.namespace, app.client)
							if err != nil {
								app.logger.Errorf("Unable to delete pod for session %s with error: %v", id, err)
							}
						})

						var reqBody map[string]interface{}

						if err := c.BodyParser(&reqBody); err == nil {
							if _, ok := reqBody["sessionId"].(string); ok {
								reqBody["sessionId"] = sess.ID
								reqBodyChanged, _ := json.Marshal(reqBody)
								r.Header.SetContentLength(len(reqBodyChanged))
								r.SetBody(reqBodyChanged)
							}
						}
					}
					c.Request().URI().SetHost(sess.URL.Host)
					c.Path(path.Clean(path.Join(sess.URL.Path, strings.Join(fragments[5:], "/"))))
					app.logger.Info("Proxy session")
					return nil
				}
				app.logger.Errorf("Unknown session %s", id)
				return fmt.Errorf("bad session id %s", id)
			},
			HandleError: func(c *fiber.Ctx) error {
				return c.SendStatus(fiber.StatusBadGateway)
			},
			ModifyResponse: func(c *fiber.Ctx) error {
				resp := c.Response()
				respBody := resp.Body()
				if respBody != nil {
					var respMsg map[string]interface{}
					if err := json.Unmarshal(respBody, &respMsg); err == nil {
						if _, ok := respMsg["sessionId"].(string); ok {
							respMsg["sessionId"] = id
							bodyChanged, _ := json.Marshal(respMsg)
							resp.Header.SetContentLength(len(bodyChanged))
							resp.SetBody(bodyChanged)
							return nil
						}
						return err
					}
				}
				return nil
			},
		}).Proxy(ctx)
	} else {
		app.logger.Error("Unable to find session")
		return respondWithWebDriverError("WebDriver session is not found", "Unable to process request because sessionId is not found", fiber.StatusNotFound, ctx)
	}

}

//HandleDevTools ...
func (app *App) HandleDevTools(ctx *fiber.Ctx) error {
	return app.proxy(ctx, ports.Devtools)
}

//HandleDownload ...
func (app *App) HandleDownload(ctx *fiber.Ctx) error {
	return app.proxy(ctx, ports.Fileserver)
}

//HandleClipboard ..
func (app *App) HandleClipboard(ctx *fiber.Ctx) error {
	return app.proxy(ctx, ports.Clipboard)
}

func (app *App) proxy(ctx *fiber.Ctx, port string) error {

	id := ctx.Params("sessionId")

	if id == "" {
		app.logger.Errorf("Session id not found")
		return respondWithWebDriverError("WebDriver session is not found", "Unable to process request because sessionId is not found", fiber.StatusNotFound, ctx)
	}

	fragments := strings.Split(ctx.Path(), "/")
	remainingPath := "/" + strings.Join(fragments[3:], "/")
	_, ok := app.bucket.get(id)

	if ok {
		return (&httpreverseproxy.ReverseProxy{
			PrepareRequest: func(c *fiber.Ctx) error {
				c.Request().URI().SetScheme("http")
				c.Request().URI().SetHost(net.JoinHostPort(app.hostname, port))
				c.Path(remainingPath)
				app.logger.Infof("proxying %s", fragments[1])
				return nil
			},
			HandleError: func(c *fiber.Ctx) error {
				app.logger.Errorf("proxying %s error", fragments[1])
				return c.SendStatus(fiber.StatusBadGateway)
			},
		}).Proxy(ctx)
	} else {
		app.logger.Errorf("Unknown session: %s", id)
		return respondWithWebDriverError("WebDriver session is not found", "Unable to process request because sessionId is not found", fiber.StatusNotFound, ctx)
	}
}

func onTimeout(t time.Duration, f func()) chan struct{} {
	cancel := make(chan struct{})
	go func(cancel chan struct{}) {
		select {
		case <-time.After(t):
			f()
		case <-cancel:
		}
	}(cancel)

	return cancel
}

func respondWithWebDriverError(errorDesc, message string, statusCode int, ctx *fiber.Ctx) error {
	return ctx.Status(statusCode).JSON(fiber.Map{
		"value": map[string]string{
			"error":   errorDesc,
			"message": message,
		},
	})
}
