package main

import (
    "os"
    "log"
    "flag"
    "net/http"
    "fmt"
    "bytes"
    "time"
    "strings"
    "strconv"
    "encoding/json"
    "io/ioutil"
    _ "mysql"
    "database/sql"
    "goproxy"
)

var (
    // save to mysql database DSN
    mysql_conn = os.Getenv("WYDSN")

    // request.Body temp var
    RequestBodyMap = make(map[int64][]byte)

    // http static resource file extension
    static_ext []string = []string{
        "js", 
        "css", 
        "ico",
    }

    // media resource files type
    media_types []string = []string{
        "image",
        "video",
        "audio",
    }

    // http static resource files
    static_types []string = []string{
        "text/css",
        // "application/javascript",
        // "application/x-javascript",
        "application/msword",
        "application/vnd.ms-excel",
        "application/vnd.ms-powerpoint",
        "application/x-ms-wmd",
        "application/x-shockwave-flash",
    }
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
    port char(6) DEFAULT NULL,
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
    PRIMARY KEY (id)
) ENGINE=MyISAM DEFAULT CHARSET=utf8`

type Response struct {
    ID            uint          `json:"id" db:",omitempty,json"`
    Origin        string        `json:"origin" db:",json"`
    Method        string        `json:"method" db:",json"`
    Status        int           `json:"status" db:",json"`
    ContentType   string        `json:"content_type" db:",json"`
    ContentLength uint          `json:"content_length" db:",json"`
    Host          string        `json:"host" db:",json"`
    Port          string        `json:"port" db:",json"`
    URL           string        `json:"url" db:",json"`
    Scheme        string        `json:"scheme" db:",json"`
    Path          string        `json:"path" db:",path"`
    Extension     string        `json:"ext" db:",path"`
    Header        http.Header   `json:"header,omitempty" db:",json"`
    Body          []byte        `json:"body,omitempty" db:",json"`
    RequestHeader http.Header   `json:"request_header,omitempty" db:",json"`
    RequestBody   []byte        `json:"request_body,omitempty" db:",json"`
    DateStart     time.Time     `json:"date_start" db:",json"`
    DateEnd       time.Time     `json:"date_end" db:",json"`
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

    var (
        ctype string
        clength int
        StrHost string
        StrPort string
    )
    
    if len(parser.r.Header["Content-Type"]) >= 1 {
        ctype = parser.r.Header["Content-Type"][0]
    }

    if len(parser.r.Header["Content-Length"]) >= 1 {
        clength, _ = strconv.Atoi(parser.r.Header["Content-Length"][0])
    }

    SliceHost := strings.Split(parser.r.Request.URL.Host, ":")
    if len(SliceHost) > 1 {
        StrHost, StrPort = SliceHost[0], SliceHost[1]
    } else {
        StrHost = SliceHost[0]
        if parser.r.Request.URL.Scheme == "https" {
            StrPort = "443"
        } else {
            StrPort = "80"
        }
    }

    now := time.Now()

    r := Response{
        Origin:        parser.r.Request.RemoteAddr,
        Method:        parser.r.Request.Method,
        Status:        parser.r.StatusCode,
        ContentType:   string(ctype),
        ContentLength: uint(clength),
        Host:          StrHost,
        Port:          StrPort,
        URL:           parser.r.Request.URL.String(),
        Scheme:        parser.r.Request.URL.Scheme,
        Path:          parser.r.Request.URL.Path,
        Extension:     GetExtension(parser.r.Request.URL.Path),
        Header:        parser.r.Header,
        Body:          parser.respbody,
        RequestHeader: parser.r.Request.Header,
        RequestBody:   parser.reqbody,
        DateStart:     parser.s,
        DateEnd:       now,
    }
    return r
}

func New(resp *http.Response, reqbody []byte, respbody []byte) *ParserHTTP {
    return &ParserHTTP{r: resp, reqbody: reqbody, respbody: respbody, s: time.Now()}
}

type ResType struct {
    ext     string
    ctype   string
    mtype   string
}

func (r *ResType) isStatic() bool {
    if ContainsString(static_ext, r.ext) {
        return true
    } else if ContainsString(static_types, r.ctype) {
        return true
    } else if ContainsString(media_types, r.mtype) {
        return true
    }
    return false
}

func ContainsString(sl []string, v string) bool {
    for _, vv := range sl {
        if vv == v {
            return true
        }
    }
    return false
}

func GetContentType(HeradeCT string) string {
    ct := strings.Split(HeradeCT, "; ")[0]
    return ct
}

func GetExtension(path string) string {
    SlicePath := strings.Split(path, ".")
    if len(SlicePath) > 1 {
        return SlicePath[len(SlicePath)-1]
    }
    return ""
}

func NewResType(ext string, ctype string) *ResType {
    var mtype string
    if ctype != "" {
        mtype = strings.Split(ctype, "/")[0]
    }
    return &ResType{ext, ctype, mtype}
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

    // js, _ := json.Marshal(RespCapture.RequestHeader)
    // fmt.Println(string(js))

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



