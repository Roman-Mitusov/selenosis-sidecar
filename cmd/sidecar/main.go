package main

import (
	"context"
	sidecar "github.com/Roman-Mitusov/selenosis-sidecar"
	"github.com/Roman-Mitusov/selenosis-sidecar/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var buildVersion = "HEAD"

func command() *cobra.Command {

	var (
		listhenPort     string
		browserPort     string
		proxyPath       string
		namespace       string
		idleTimeout     time.Duration
		shutdownTimeout time.Duration
		shuttingDown    bool
	)

	cmd := &cobra.Command{
		Use:   "selenosis sidecar proxy based on fiber router",
		Short: "sidecar proxy for selenosis",
		Run: func(cmd *cobra.Command, args []string) {
			var err error
			log := logrus.New()
			log.Formatter = &logrus.JSONFormatter{}

			hostname, err := os.Hostname()
			if err != nil {
				log.Fatalf("Can't get container hostname: %v", err)
			}

			log.Infof("Starting selenosis sidecar proxy %s", buildVersion)

			client, err := utils.BuildClusterClient()
			if err != nil {
				log.Fatalf("Failed to build kubernetes client: %v", err)
			}

			ctx := context.Background()
			_, err = client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
			if err != nil {
				log.Fatalf("Failed to get namespace: %s: %v", namespace, err)
			}

			log.Info("Kubernetes client successfully created")

			storage := sidecar.NewStorage()

			app := sidecar.New(&sidecar.Config{
				BrowserPort:     browserPort,
				ProxyPath:       proxyPath,
				Hostname:        hostname,
				Namespace:       namespace,
				IdleTimeout:     idleTimeout,
				ShutdownTimeout: shutdownTimeout,
				Storage:         storage,
				Client:          client,
				Logger:          log,
			})

			router := fiber.New(fiber.Config{
				DisableStartupMessage: true,
				StrictRouting:         true,
			})

			router.Use(recover.New(), requestid.New(), logger.New(logger.Config{
				Format: "[${time}] | request_id: ${locals:requestid} | status_code: ${status} | http_method: ${method} | client_ip: ${ip} | path: ${path} | request_body: ${body} | response: ${resBody}\n",
			}))

			router.Post("/wd/hub/session", app.HandleSessionCreate)
			router.All("/wd/hub/session/:sessionId/*", app.HandleWebDriverRequests)
			router.All("/devtools/:sessionId", app.HandleDevTools)
			router.All("/download/:sessionId", app.HandleDownload)
			router.All("/clipboard/:sessionId", app.HandleClipboard)
			router.Get("/status", func(ctx *fiber.Ctx) error {
				if shuttingDown {
					return ctx.SendStatus(fiber.StatusBadGateway)
				} else {
					return ctx.SendStatus(fiber.StatusOK)
				}
			})

			stop := make(chan os.Signal)
			signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGKILL, os.Interrupt)

			e := make(chan error)

			go func() {
				_ = <-stop
				log.Info("Gracefully shutting down...")
				_ = router.Shutdown()
			}()

			go func() {
				e <- router.Listen(":" + listhenPort)
			}()

			go func() {
				timeout := time.After(idleTimeout)
				ticker := time.Tick(500 * time.Millisecond)
			loop:
				for {
					select {
					case <-timeout:
						ctx := context.Background()
						_ = client.CoreV1().Pods(namespace).Delete(ctx, hostname, metav1.DeleteOptions{
							GracePeriodSeconds: pointer.Int64Ptr(15),
						})
						log.Warn("Session wait timeout exceeded")
						break loop
					case <-ticker:
						if storage.IsEmpty() {
							break
						}
						break loop
					}
				}
			}()

			select {
			case err := <-e:
				log.Fatalf("Failed to start selenosis sidecar proxy: %v", err)
			case <-stop:
				shuttingDown = true
				log.Info("Stopping selenosis sidecar proxy")
			}
		},
	}

	cmd.Flags().StringVar(&listhenPort, "listhen-port", "4445", "port to use for incomming requests")
	cmd.Flags().StringVar(&browserPort, "browser-port", "4444", "browser port")
	cmd.Flags().StringVar(&proxyPath, "proxy-default-path", "/session", "path used by handler")
	cmd.Flags().DurationVar(&idleTimeout, "idle-timeout", 120*time.Second, "time in seconds for idle session")
	cmd.Flags().StringVar(&namespace, "namespace", "selenosis", "kubernetes namespace")
	cmd.Flags().DurationVar(&shutdownTimeout, "graceful-shutdown-timeout", 15*time.Second, "time in seconds  gracefull shutdown timeout")

	cmd.Flags().SortFlags = false

	return cmd
}

func main() {
	if err := command().Execute(); err != nil {
		os.Exit(1)
	}
}
