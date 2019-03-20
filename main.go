package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/DITAS-Project/tub-mock-dal/dal"

	"github.com/gorilla/mux"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
)

func init() {

}

func cliSetup() {
	viper.SetDefault("port", 8080)

	flag.Int("port", 8080, "set the port to listen to")
	flag.Bool("trace", false, "use trace headers")

	flag.String("dal", "", "dal address to use, useing fake data if empty")
	flag.String("log", "", "log agent address to use, dont log if empty")

	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()
	viper.BindPFlags(pflag.CommandLine)

}

func main() {
	cliSetup()

	setupServer()
}

func setupServer() {
	vdcServer := New()

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", viper.GetInt("port")),
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      vdcServer.Router(),
	}
	fmt.Printf("started mock-vdc running at %d\n", viper.GetInt("port"))
	server.ListenAndServe()
}

type VDCServer struct {
	log    bool
	logUri string
	trace  bool

	dal    bool
	dalUri string

	dalClient dal.DalClient
	conn      *grpc.ClientConn
}

func New() *VDCServer {

	server := &VDCServer{
		log:    viper.GetString("log") != "",
		logUri: viper.GetString("log"),
		trace:  (viper.GetString("log") != "" && viper.GetBool("trace")),
		dal:    viper.GetString("dal") != "",
		dalUri: viper.GetString("dal"),
	}

	if server.dal {

		conn, err := grpc.Dial(server.dalUri, grpc.WithInsecure())

		if err != nil {
			fmt.Printf("could not connect to dal %+v\n", err)
			os.Exit(-1)
		}

		server.conn = conn

		client := dal.NewDalClient(conn)
		server.dalClient = client

	}

	return server
}

func (v *VDCServer) Clean() {
	if v.conn != nil {
		v.conn.Close()
	}
}

func (v *VDCServer) Router() *mux.Router {
	r := mux.NewRouter()

	r.Methods("GET").Path("/ask").Handler(http.HandlerFunc(v.ask))
	r.NotFoundHandler = http.HandlerFunc(v.notFound)
	return r
}

type LogMessage struct {
	Value string `json:"value,omitempty"`
}

type TraceMessage struct {
	TraceId      string `json:"traceId"`
	ParentSpanId string `json:"parentSpanId"`
	SpanId       string `json:"spanId"`
	Operation    string `json:"operation"`
	Message      string `json:"message"`
}

func (v *VDCServer) Log(msg string) {
	if v.log {
		logMsg := LogMessage{Value: msg}
		b := new(bytes.Buffer)
		json.NewEncoder(b).Encode(logMsg)
		http.Post(fmt.Sprintf("%s/v1/log", v.logUri), "application/json", b)
	}

	fmt.Println("[Log]", msg)

}

func (v *VDCServer) traceFromRequest(r *http.Request, msg ...string) TraceMessage {
	trace := TraceMessage{}

	trace.ParentSpanId = r.Header.Get("X-B3-SpanId")
	trace.TraceId = r.Header.Get("X-B3-TraceId")
	trace.Operation = "vdc-processing"
	trace.Message = strings.Join(msg, " ")

	return trace
}

func (v *VDCServer) send(url string, msg TraceMessage) {
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(msg)
	http.Post(url, "application/json", b)
}

func (v *VDCServer) Trace(r *http.Request, msg ...string) {
	if v.trace {
		trace := v.traceFromRequest(r, msg...)
		v.send(fmt.Sprintf("%s/v1/trace", v.logUri), trace)
	}

	fmt.Println("[Trace]", msg)
}

func (v *VDCServer) TraceClose(r *http.Request, msg ...string) {
	if v.trace {
		trace := v.traceFromRequest(r, msg...)
		v.send(fmt.Sprintf("%s/v1/close", v.logUri), trace)
	}

	fmt.Println("[Close]", msg)
}

func (v *VDCServer) SendDalResponse(w http.ResponseWriter, r *http.Request) {
	v.Trace(r, "vdc-dal-request")

	result, err := v.dalClient.Query(context.Background(), &dal.QueryRequest{Sql: "Select * From Messages"})

	v.Trace(r, "vdc-dal-response")

	if err != nil {
		v.Log(fmt.Sprintf("failed to call dal %+v", err))
		v.SendMockResponse(w, r)
		return
	}

	if result.Error != nil {
		v.Log(fmt.Sprintf("internal dal error %s", result.Error.Message))
		v.SendMockResponse(w, r)
		return
	}

	vals := result.Result.Result["msg"]
	msg := vals.Value[0]

	w.Write([]byte(fmt.Sprintf("{\"mgs\":\"%s\"}", msg)))

}

func (v *VDCServer) SendMockResponse(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("{\"mgs\":\"Hello World\"}"))

}

func (v *VDCServer) notFound(w http.ResponseWriter, r *http.Request) {
	v.Log(fmt.Sprintf("[%s] %s", r.Method, r.RequestURI))

	w.Write([]byte("{\"msg\":\"content not found\"}"))
	w.WriteHeader(http.StatusNotFound)
}

func (v *VDCServer) ask(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS, HEAD")
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization, X-DITAS-CALLBACK")

	v.Log(fmt.Sprintf("[%s] %s", r.Method, r.RequestURI))
	v.Trace(r, "vdc-request")

	data, _ := ioutil.ReadAll(r.Body)

	v.Log(fmt.Sprintf("- %s ", string(data)))

	w.Header().Set("Content-Type", "application/json")
	if v.dal {
		v.SendDalResponse(w, r)
	} else {
		v.SendMockResponse(w, r)
	}
	w.WriteHeader(http.StatusOK)

	v.TraceClose(r, "vdc-request")
}
