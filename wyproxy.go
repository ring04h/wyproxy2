package main

import (
    "os"
    "log"
    "flag"
    "net/http"
    "fmt"
    "bytes"
    "time"
    "encoding/json"
    "io/ioutil"
    _ "mysql"
    "database/sql"
    "goproxy"
)

var (
    mysql_conn = os.Getenv("WYDSN")
    RequestBodyMap = make(map[int64][]byte)
)

const (
    version = "0.1"
    default_mysql_conn = "root:@tcp(localhost:3306)/test?charset=utf8"
    default_table      = `capture`
)

const tableCreateSQL = `CREATE TABLE if not exists ` + default_table + ` (
    id int(10) unsigned NOT NULL AUTO_INCREMENT,
    static_resource tinyint(1) DEFAULT NULL,
    method char(10) DEFAULT NULL,
    status_code int(6) DEFAULT NULL,
    content_type varchar(50) DEFAULT NULL,
    content_length int(11) DEFAULT NULL,
    host varchar(255) DEFAULT NULL,
    url text,
    scheme char(10) DEFAULT NULL,
    path text,
    header mediumtext,
    content mediumblob,
    request_header mediumtext,
    request_content mediumblob,
    date_start datetime DEFAULT NULL,
    date_end datetime DEFAULT NULL,
    extension char(32) DEFAULT NULL,
    port char(6) DEFAULT NULL,
    PRIMARY KEY (id)
) ENGINE=MyISAM DEFAULT CHARSET=utf8`

type Header struct {
    http.Header
}

type Response struct {
    ID            uint      `json:"id" db:",omitempty,json"`
    Origin        string    `json:"origin" db:",json"`
    Method        string    `json:"method" db:",json"`
    Status        int       `json:"status" db:",json"`
    ContentType   string    `json:"content_type" db:",json"`
    ContentLength uint      `json:"content_length" db:",json"`
    Host          string    `json:"host" db:",json"`
    URL           string    `json:"url" db:",json"`
    Scheme        string    `json:"scheme" db:",json"`
    Path          string    `json:"path" db:",path"`
    Header        Header    `json:"header,omitempty" db:",json"`
    Body          []byte    `json:"body,omitempty" db:",json"`
    RequestHeader Header    `json:"request_header,omitempty" db:",json"`
    RequestBody   []byte    `json:"request_body,omitempty" db:",json"`
    DateStart     time.Time `json:"date_start" db:",json"`
    DateEnd       time.Time `json:"date_end" db:",json"`
    TimeTaken     int64     `json:"time_taken" db:",json"`
}

func (h Header) MarshalDB() (interface{}, error) {
    return json.Marshal(h.Header)
}

func (h *Header) UnmarshalDB(data interface{}) error {
    if s, ok := data.(string); ok {
        return json.Unmarshal([]byte(s), &h.Header)
    }
    return nil
}

func init() {
    if mysql_conn == "" {
        mysql_conn = default_mysql_conn
    }
    dbsetup()
}

func dbsetup() {
    db, err := sql.Open("mysql", mysql_conn)
    checkErr(err)
    _, err = db.Query(tableCreateSQL)
    checkErr(err)
    db.Close()
}

func checkErr(err error) {
    if err != nil {
        panic(err)
    }
}

type ParserHTTP struct {
    r           *http.Response
    reqbody     []byte
    respbody    []byte
    s           time.Time
}

func (parser *ParserHTTP) Parser() Response {
    now := time.Now()

    r := Response{
        Origin:        parser.r.Request.RemoteAddr,
        Method:        parser.r.Request.Method,
        Status:        parser.r.StatusCode,
        ContentType:   http.DetectContentType(parser.respbody),
        ContentLength: uint(len(parser.respbody)),
        Host:          parser.r.Request.URL.Host,
        URL:           parser.r.Request.URL.String(),
        Scheme:        parser.r.Request.URL.Scheme,
        Path:          parser.r.Request.URL.Path,
        Header:        Header{parser.r.Header},
        Body:          parser.respbody,
        RequestHeader: Header{parser.r.Request.Header},
        RequestBody:   parser.reqbody,
        DateStart:     parser.s,
        DateEnd:       now,
        TimeTaken:     now.UnixNano() - parser.s.UnixNano(),
    }
    return r
}

func New(resp *http.Response, reqbody []byte, respbody []byte) *ParserHTTP {
    return &ParserHTTP{r: resp, reqbody: reqbody, respbody: respbody, s: time.Now()}
}

func RequestBody(res *http.Request) ([]byte, error) {
    buf, err := ioutil.ReadAll(res.Body)
    if err != nil {
        return nil, err
    }
    res.Body = ioutil.NopCloser(bytes.NewReader(buf))
    return buf, nil
}

func ResponseBody(res *http.Response) ([]byte, error) {
    buf, err := ioutil.ReadAll(res.Body)
    if err != nil {
        return nil, err
    }
    res.Body = ioutil.NopCloser(bytes.NewReader(buf))
    return buf, nil
}

func printHeader(header http.Header) {
    var headers string
    for k, v := range header {
        headers = headers + fmt.Sprintf("    %s: %s\n", k, v)
    }
    log.Printf("headers:\n%s", headers)
}

func handleRequest(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
    reqbody, err := RequestBody(req)
    checkErr(err)
    RequestBodyMap[ctx.Session] = reqbody
    return req, nil
}

func handleResponse(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
    // Getting the Body
    reqbody := RequestBodyMap[ctx.Session]
    respbody, err := ResponseBody(resp)
    checkErr(err)
    delete(RequestBodyMap, ctx.Session)

    // Attaching capture tool.
    RespCapture := New(resp, reqbody, respbody).Parser()
    // fmt.Println(RespCapture)

    b, err := json.Marshal(RespCapture)
    checkErr(err)
    fmt.Println(string(b))

    // printHeader(resp.Header)
    // fmt.Println(resp.StatusCode, resp.Proto, resp.ContentLength, resp.TransferEncoding)
    // fmt.Println(resp.Request.URL, resp.Request.Method, resp.Request.Host, resp.Request.RemoteAddr)
    // fmt.Println(resp.Request.PostForm)
    return resp
}

func main() {

    verbose := flag.Bool("v", false, "should every proxy request be logged to stdout")
    addr := flag.String("addr", ":8080", "proxy listen address")
    flag.Parse()

    proxy := goproxy.NewProxyHttpServer()
    proxy.OnRequest().HandleConnect(goproxy.AlwaysMitm)

    proxy.OnRequest().DoFunc(handleRequest)
    proxy.OnResponse().DoFunc(handleResponse)

    proxy.Verbose = *verbose
    log.Fatal(http.ListenAndServe(*addr, proxy))
}



