package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/caddyserver/certmagic"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-fuego/fuego"
	"github.com/rs/cors"
)

type Received struct {
	Name string `json:"name" validate:"required"`
}

type MyResponse struct {
	Message       string `json:"message"`
	BestFramework string `json:"best"`
}

var domainName string

func main() {
	flag.StringVar(&domainName, "d", "", "domain name to use for TLS")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	certmagic.DefaultACME.Agreed = true
	certmagic.DefaultACME.Email = fmt.Sprintf("webmaster@%s", domainName)
	certmagic.DefaultACME.CA = certmagic.LetsEncryptStagingCA
	magic := certmagic.NewDefault()
	myACME := certmagic.NewACMEIssuer(magic, certmagic.DefaultACME)

	go func() {
		if err := http.ListenAndServe(fmt.Sprintf("%s:80", domainName), myACME.HTTPChallengeHandler(http.NewServeMux())); err != nil {
			log.Println("http listener error: ", err)
		}
	}()

	tlsConfig := magic.TLSConfig()
	// be sure to customize NextProtos if serving a specific
	// application protocol after the TLS handshake, for example:
	tlsConfig.NextProtos = append([]string{"h2", "http/1.1"}, tlsConfig.NextProtos...)

	s := fuego.NewServer(
		fuego.WithAddr(fmt.Sprintf("%s:443", domainName)),
		fuego.WithTLSConfig(tlsConfig),
	)

	fuego.Use(s, cors.Default().Handler)
	//fuego.Use(s, myACME.HTTPChallengeHandler)
	fuego.Use(s, chiMiddleware.Compress(5, "text/html", "text/css"))

	// Fuego 🔥 handler with automatic OpenAPI generation, validation, (de)serialization and error handling
	fuego.Post(s, "/", func(c *fuego.ContextWithBody[Received]) (MyResponse, error) {
		data, err := c.Body()
		if err != nil {
			return MyResponse{}, err
		}

		// read the request header test
		if c.Request().Header.Get("test") != "test" {
			return MyResponse{}, errors.New("test header not found")
		}

		c.Response().Header().Set("X-Hello", "World")

		return MyResponse{
			Message:       "Hello, " + data.Name,
			BestFramework: "Fuego!",
		}, nil
	}).
		Description("Say hello to the world").
		Header("test", "Just a test header").
		Cookie("test", "A Cookie!")

	// Standard net/http handler with automatic OpenAPI route declaration
	fuego.GetStd(s, "/std", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("Hello, World!"))
	})

	go func() {
		log.Printf("server listening on %s\n", s.Server.Addr)
		if err := s.Run(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server run error: %v", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Server.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutodwn error: %v", err)
	}
	log.Println("Server stopped")
}

// InTransform will be called when using c.Body().
// It can be used to transform the entity and raise custom errors
func (r *Received) InTransform(context.Context) error {
	r.Name = strings.ToLower(r.Name)
	if r.Name == "fuego" {
		return errors.New("fuego is not a name")
	}
	return nil
}

// OutTransform will be called before sending data
func (r *MyResponse) OutTransform(context.Context) error {
	r.Message = strings.ToUpper(r.Message)
	return nil
}

var (
	_ fuego.InTransformer  = &Received{}   // Ensure that *Received implements fuego.InTransformer
	_ fuego.OutTransformer = &MyResponse{} // Ensure that *MyResponse implements fuego.OutTransformer
)
