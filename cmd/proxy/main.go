package main

import (
	"encoding/json"
	"flag"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/rs/zerolog"
	"io"
	"net/http"
	"os"
	"strconv"
)
import "github.com/rs/zerolog/log"
import "github.com/go-resty/resty/v2"

func main() {

	logger := log.Logger

	apiKey := flag.String("apikey", "", "Used in upstream requests to the SkyMavis API.")
	address := flag.String("address", "0.0.0.0:3000", "The address this proxy should bind to. Format: IPv4:Port")
	logFilePath := flag.String("log-file", "llamaproxy.log", "The log file to store request and response. An empty value will disable this log.")
	flag.Parse()

	if *apiKey == "" {
		log.Fatal().Msg("apikey must not be empty")
	}

	e := echo.New()
	e.Use(middleware.RequestID())
	e.Use(middleware.Logger())

	if *logFilePath != "" {
		logFile, err := os.OpenFile(*logFilePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
		if err != nil {
			log.Fatal().Msgf("Failed to access log file: %s", err.Error())
		}
		logOut := zerolog.MultiLevelWriter(logFile)
		logger = zerolog.New(logOut).With().Timestamp().Logger()

		e.Use(middleware.BodyDump(func(c echo.Context, reqBody, resBody []byte) {
			var input json.RawMessage
			_ = json.Unmarshal(reqBody, &input)
			var output json.RawMessage
			_ = json.Unmarshal(resBody, &output)

			data := map[string]interface{}{
				"id":         c.Response().Header().Get(echo.HeaderXRequestID),
				"remote_ip":  c.RealIP(),
				"host":       c.Request().Host,
				"method":     c.Request().Method,
				"uri":        c.Request().RequestURI,
				"user_agent": c.Request().UserAgent(),
				"status":     strconv.Itoa(c.Response().Status),
				"request":    input,
				"response":   output,
			}

			out, _ := json.Marshal(data)

			logger.Info().RawJSON("data", out).Msg(strconv.Itoa(c.Response().Status))
		}))
	}

	e.HideBanner = true
	e.HidePort = true

	client := resty.New().
		SetHeader("X-API-KEY", *apiKey).
		SetHeader("Content-Type", "application/json").
		SetBaseURL("https://api-gateway.skymavis.com")

	e.POST("/graphql/katana", func(c echo.Context) error {

		client.SetHeader("Accept", c.Request().Header.Get("Accept")).
			SetHeader("Accept-Language", c.Request().Header.Get("Accept-Language")).
			SetHeader("Connection", c.Request().Header.Get("Connection")).
			SetHeader("Host", c.Request().Header.Get("Host")).
			SetHeader("Content-Type", "application/json").
			SetHeader("User-Agent", c.Request().Header.Get("User-Agent"))

		body, err := io.ReadAll(c.Request().Body)

		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": err.Error(),
			})
		}

		var result interface{}

		request, err := client.
			R().
			SetBody(body).
			SetResult(&result).
			Post("/graphql/katana")

		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": err.Error(),
			})
		}

		return c.JSON(request.StatusCode(), result)
	})

	err := e.Start(*address)

	if err != nil {
		log.Print("Something went wrong")
	}
}
