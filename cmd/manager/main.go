package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/atomix/chaos-controller/pkg/chaos"
	"os"
	"runtime"

	"github.com/atomix/chaos-controller/pkg/apis"
	"github.com/atomix/chaos-controller/pkg/controller"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"github.com/operator-framework/operator-sdk/pkg/leader"
	"github.com/operator-framework/operator-sdk/pkg/ready"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

var log = logf.Log.WithName("cmd")

func printVersion() {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	log.Info(fmt.Sprintf("operator-sdk Version: %v", sdkVersion.Version))
}

func main() {
	flag.Parse()

	// The logger instantiated here can be changed to any logger
	// implementing the logr.Logger interface. This logger will
	// be propagated through the whole operator, generating
	// uniform and structured logs.
	logf.SetLogger(logf.ZapLogger(false))

	printVersion()

	namespace, err := k8sutil.GetWatchNamespace()
	if err != nil {
		log.Error(err, "failed to get watch namespace")
		os.Exit(1)
	}

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	wait := make(chan bool)
	go (func() {
		// Become the leader before proceeding
		leader.Become(context.TODO(), "chaos-operator-lock")

		r := ready.NewFileReady()
		err = r.Set()
		if err != nil {
			log.Error(err, "")
			os.Exit(1)
		}
		defer r.Unset()

		// Create a new Cmd to provide shared dependencies and start components
		mgr, err := manager.New(cfg, manager.Options{Namespace: namespace})
		if err != nil {
			log.Error(err, "")
			os.Exit(1)
		}

		log.Info("Registering Components.")

		// Setup Scheme for all resources
		if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
			log.Error(err, "")
			wait<-true
		}

		// Setup all Controllers
		if err := controller.AddController(mgr); err != nil {
			log.Error(err, "")
			wait<-true
		}

		log.Info("Starting the Cmd.")

		// Start the Cmd
		if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
			log.Error(err, "manager exited non-zero")
			wait<-true
		}
	})()

	go (func() {
		// Create a new Cmd to provide shared dependencies and start components
		mgr, err := manager.New(cfg, manager.Options{Namespace: namespace})
		if err != nil {
			log.Error(err, "")
			os.Exit(1)
		}

		log.Info("Registering Components.")

		// Setup Scheme for all resources
		if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
			log.Error(err, "")
			os.Exit(1)
		}

		// Setup all Controllers
		if err := chaos.AddControllers(mgr); err != nil {
			log.Error(err, "")
			wait<-true
		}

		log.Info("Starting the Cmd.")

		// Start the Cmd
		if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
			log.Error(err, "manager exited non-zero")
			wait<-true
		}
	})()
	<-wait
	os.Exit(1)
}
