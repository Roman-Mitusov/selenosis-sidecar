package sidecar

import (
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"time"
)

//Config basic config
type Config struct {
	BrowserPort     string
	ProxyPath       string
	Hostname        string
	Namespace       string
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
	Storage         *Storage
	Client          *kubernetes.Clientset
	Logger          *logrus.Logger
}

//App ...
type App struct {
	browserPort     string
	proxyPath       string
	hostname        string
	namespace       string
	idleTimeout     time.Duration
	shutdownTimeout time.Duration
	bucket          *Storage
	client          *kubernetes.Clientset
	logger          *logrus.Logger
}

//New ...
func New(conf *Config) *App {
	return &App{
		browserPort:     conf.BrowserPort,
		proxyPath:       conf.ProxyPath,
		hostname:        conf.Hostname,
		namespace:       conf.Namespace,
		idleTimeout:     conf.IdleTimeout,
		shutdownTimeout: conf.ShutdownTimeout,
		bucket:          conf.Storage,
		client:          conf.Client,
		logger:          conf.Logger,
	}
}
