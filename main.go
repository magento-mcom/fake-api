package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/magento-mcom/fake-api/api"
	"github.com/magento-mcom/fake-api/api/handler"
	"github.com/magento-mcom/fake-api/app"
	"github.com/magento-mcom/fake-api/order"
	"github.com/satori/go.uuid"
	"gopkg.in/gin-gonic/gin.v1"
	"gopkg.in/yaml.v2"
)

func main() {
	var configFile string
	flag.StringVar(&configFile, "config", "", "Config file")
	flag.Parse()

	content, err := ioutil.ReadFile(configFile)

	if err != nil {
		panic(fmt.Sprintf("Error reading config file: %v", err))
	}

	config := app.Config{}

	if err = yaml.Unmarshal(content, &config); err != nil {
		panic(fmt.Sprintf("Error composing configuration: %v", err))
	}

	router := gin.New()

	r := api.NewRegistry()
	p := api.NewPublisher(r)
	or := order.NewOrderRegistry()
	mh := map[string]api.Handler{
		"magento.service_bus.remote.register":              handler.NewRegisterHandler(r),
		"magento.sales.order_management.create":            handler.NewCreateOrderHandler(p, config.StatusToExport, or),
		"magento.inventory.source_stock_management.update": handler.NewSourceUpdateHandler(p, config.AggregatesToExport),
	}

	d := api.NewDispatcher(mh)

	router.POST("/api", func(ctx *gin.Context) {
		data := api.Request{}

		b, err := ioutil.ReadAll(ctx.Request.Body)
		if err != nil {
			return
		}

		if err := json.Unmarshal(b, &data); err != nil {
			fmt.Printf("%v", err)
			return
		}

		res, err := d.Dispatch(data)

		respBody := api.Response{
			ID:      data.ID,
			JsonRpc: "2.0",
		}

		if err == nil {
			m, err := json.Marshal(res)
			if err == nil {
				raw := json.RawMessage(m)
				respBody.Result = &raw
			}

		}

		if err != nil {
			respBody.Error = err.Error()
		}

		ctx.JSON(http.StatusOK, respBody)
	})

	router.POST("/order/:id", func(ctx *gin.Context) {
		orderId := ctx.Param("id")

		id, _ := uuid.NewV4()
		respBody := api.Response{
			ID:      id.String(),
			JsonRpc: "2.0",
		}

		if !or.Exists(orderId) {
			respBody.Error = fmt.Sprintf("Order with id %v not exists.", orderId)
		}

		ctx.JSON(http.StatusOK, respBody)
	})

	srv := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       10 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		Addr:              ":24213",
		Handler:           router,
	}

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
}
