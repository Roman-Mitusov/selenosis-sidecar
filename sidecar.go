package sidecar

import (
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
	}
}
