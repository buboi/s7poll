package main

import (
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unsafe"

	gos7 "github.com/robinson/gos7"
)

type connOptions struct {
	address string
	rack    int
	slot    int
	ctype   uint
	port    int
}

type areaOptions struct {
	area   string
	db     int
	start  int
	amount int
}

type closeHandler interface {
	Close() error
}

type plcClient struct {
	Client  gos7.Client
	handler closeHandler
}

func (c *plcClient) Close() {
	if c == nil || c.handler == nil {
		return
	}
	_ = c.handler.Close()
}

func main() {
	if len(os.Args) < 2 {
		printGlobalUsage()
		return
	}

	switch os.Args[1] {
	case "read":
		if err := runRead(os.Args[2:]); err != nil {
			fail(err)
		}
	case "poll":
		if err := runPoll(os.Args[2:]); err != nil {
			fail(err)
		}
	case "write":
		if err := runWrite(os.Args[2:]); err != nil {
			fail(err)
		}
	case "help", "-h", "--help":
		printGlobalUsage()
	default:
		fail(fmt.Errorf("unknown command %q", os.Args[1]))
	}
}

func runRead(args []string) error {
	fs := flag.NewFlagSet("read", flag.ContinueOnError)
	conn := addConnFlags(fs)
	area := addAreaFlags(fs)
	format := fs.String("format", "hex", "Output format: hex|string|int16|int32|float32")
	if err := fs.Parse(args); err != nil {
		return err
	}

	plc, err := connect(conn)
	if err != nil {
		return err
	}
	defer plc.Close()

	data, err := readArea(plc.Client, area)
	if err != nil {
		return err
	}

	out, err := formatData(data, *format)
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}

func runPoll(args []string) error {
	fs := flag.NewFlagSet("poll", flag.ContinueOnError)
	conn := addConnFlags(fs)
	area := addAreaFlags(fs)
	format := fs.String("format", "hex", "Output format: hex|string|int16|int32|float32")
	interval := fs.Duration("interval", time.Second, "Poll interval, e.g. 500ms, 2s")
	count := fs.Int("count", 0, "Number of polls (0 = run until interrupted)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	plc, err := connect(conn)
	if err != nil {
		return err
	}
	defer plc.Close()

	i := 0
	for {
		data, err := readArea(plc.Client, area)
		if err != nil {
			return err
		}
		out, err := formatData(data, *format)
		if err != nil {
			return err
		}
		fmt.Printf("[%s] %s\n", time.Now().Format(time.RFC3339), out)

		i++
		if *count > 0 && i >= *count {
			return nil
		}
		time.Sleep(*interval)
	}
}

func runWrite(args []string) error {
	fs := flag.NewFlagSet("write", flag.ContinueOnError)
	conn := addConnFlags(fs)
	area := addAreaFlags(fs)
	format := fs.String("format", "hex", "Input format: hex|string|int16|int32|float32")
	rawValues := fs.String("values", "", "Data to write. For hex, use pairs (e.g. 01ff). For numeric formats, use comma-separated values.")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *rawValues == "" {
		return errors.New("values is required")
	}

	payload, err := parseWriteData(*format, *rawValues)
	if err != nil {
		return err
	}

	plc, err := connect(conn)
	if err != nil {
		return err
	}
	defer plc.Close()

	return writeArea(plc.Client, area, payload)
}

func addConnFlags(fs *flag.FlagSet) *connOptions {
	opts := &connOptions{}
	fs.StringVar(&opts.address, "addr", "127.0.0.1", "PLC address (IP or host)")
	fs.IntVar(&opts.rack, "rack", 0, "Rack number")
	fs.IntVar(&opts.slot, "slot", 1, "Slot number")
	fs.UintVar(&opts.ctype, "ctype", 0, "Connection type (0 for default)")
	fs.IntVar(&opts.port, "port", 102, "TCP port (default 102)")
	return opts
}

func addAreaFlags(fs *flag.FlagSet) *areaOptions {
	opts := &areaOptions{}
	fs.StringVar(&opts.area, "area", "DB", "Memory area: DB|PE|PA|MK|TM|CT")
	fs.IntVar(&opts.db, "db", 1, "DB number (used when area=DB)")
	fs.IntVar(&opts.start, "start", 0, "Start offset (byte)")
	fs.IntVar(&opts.amount, "size", 4, "Number of bytes to read/write")
	return opts
}

func connect(opts *connOptions) (*plcClient, error) {
	address := opts.address
	if !strings.Contains(address, ":") && opts.port > 0 {
		address = fmt.Sprintf("%s:%d", address, opts.port)
	}

	var handler *gos7.TCPClientHandler
	if opts.ctype > 0 {
		handler = gos7.NewTCPClientHandlerWithConnectType(address, opts.rack, opts.slot, int(opts.ctype))
	} else {
		handler = gos7.NewTCPClientHandler(address, opts.rack, opts.slot)
	}

	if err := handler.Connect(); err != nil {
		return nil, err
	}

	return &plcClient{
		Client:  gos7.NewClient(handler),
		handler: handler,
	}, nil
}

func readArea(client gos7.Client, area *areaOptions) ([]byte, error) {
	buf := make([]byte, area.amount)
	var err error
	switch strings.ToUpper(area.area) {
	case "DB":
		err = client.AGReadDB(area.db, area.start, area.amount, buf)
	case "PE", "I", "INPUT":
		err = client.AGReadEB(area.start, area.amount, buf)
	case "PA", "Q", "OUTPUT":
		err = client.AGReadAB(area.start, area.amount, buf)
	case "MK", "M", "MERKER":
		err = client.AGReadMB(area.start, area.amount, buf)
	default:
		return nil, fmt.Errorf("area %q not supported", area.area)
	}
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func writeArea(client gos7.Client, area *areaOptions, data []byte) error {
	switch strings.ToUpper(area.area) {
	case "DB":
		return client.AGWriteDB(area.db, area.start, len(data), data)
	case "PE", "I", "INPUT":
		return client.AGWriteEB(area.start, len(data), data)
	case "PA", "Q", "OUTPUT":
		return client.AGWriteAB(area.start, len(data), data)
	case "MK", "M", "MERKER":
		return client.AGWriteMB(area.start, len(data), data)
	default:
		return fmt.Errorf("area %q not supported", area.area)
	}
}

func formatData(data []byte, format string) (string, error) {
	switch strings.ToLower(format) {
	case "hex":
		return bytesToHex(data), nil
	case "string", "str":
		return string(data), nil
	case "int16":
		vals, err := decodeInts(data, 2)
		if err != nil {
			return "", err
		}
		return joinInts(vals), nil
	case "int32":
		vals, err := decodeInts(data, 4)
		if err != nil {
			return "", err
		}
		return joinInts(vals), nil
	case "float32", "float":
		if len(data)%4 != 0 {
			return "", fmt.Errorf("float32 output requires size to be a multiple of 4, got %d", len(data))
		}
		out := make([]string, 0, len(data)/4)
		for i := 0; i < len(data); i += 4 {
			v := mathFromBits(binary.BigEndian.Uint32(data[i:]))
			out = append(out, fmt.Sprintf("%g", v))
		}
		return strings.Join(out, ","), nil
	default:
		return "", fmt.Errorf("unknown format %q", format)
	}
}

func parseWriteData(format, raw string) ([]byte, error) {
	switch strings.ToLower(format) {
	case "hex":
		return parseHex(raw)
	case "string", "str":
		return []byte(raw), nil
	case "int16":
		return encodeInts(raw, 2)
	case "int32":
		return encodeInts(raw, 4)
	case "float32", "float":
		return encodeFloats(raw)
	default:
		return nil, fmt.Errorf("unknown format %q", format)
	}
}

func bytesToHex(data []byte) string {
	var b strings.Builder
	for i, v := range data {
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%02X", v)
	}
	return b.String()
}

func parseHex(s string) ([]byte, error) {
	clean := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(s, " ", ""), "0x", ""), "0X", "")
	if len(clean)%2 != 0 {
		return nil, fmt.Errorf("hex input length must be even, got %d", len(clean))
	}
	out := make([]byte, len(clean)/2)
	for i := 0; i < len(clean); i += 2 {
		var v byte
		_, err := fmt.Sscanf(clean[i:i+2], "%02X", &v)
		if err != nil {
			return nil, fmt.Errorf("invalid hex at position %d: %w", i, err)
		}
		out[i/2] = v
	}
	return out, nil
}

func decodeInts(data []byte, size int) ([]int64, error) {
	if len(data)%size != 0 {
		return nil, fmt.Errorf("size %d not aligned with data length %d", size, len(data))
	}
	vals := make([]int64, 0, len(data)/size)
	for i := 0; i < len(data); i += size {
		switch size {
		case 2:
			vals = append(vals, int64(int16(binary.BigEndian.Uint16(data[i:]))))
		case 4:
			vals = append(vals, int64(int32(binary.BigEndian.Uint32(data[i:]))))
		}
	}
	return vals, nil
}

func encodeInts(raw string, size int) ([]byte, error) {
	parts := splitValues(raw)
	out := make([]byte, 0, len(parts)*size)
	for _, p := range parts {
		val, err := parseInt(p, size)
		if err != nil {
			return nil, err
		}
		switch size {
		case 2:
			buf := make([]byte, 2)
			binary.BigEndian.PutUint16(buf, uint16(val))
			out = append(out, buf...)
		case 4:
			buf := make([]byte, 4)
			binary.BigEndian.PutUint32(buf, uint32(val))
			out = append(out, buf...)
		}
	}
	return out, nil
}

func encodeFloats(raw string) ([]byte, error) {
	parts := splitValues(raw)
	out := make([]byte, 0, len(parts)*4)
	for _, p := range parts {
		val, err := parseFloat(p)
		if err != nil {
			return nil, err
		}
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, mathToBits(val))
		out = append(out, buf...)
	}
	return out, nil
}

func splitValues(raw string) []string {
	items := strings.Split(raw, ",")
	res := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			res = append(res, item)
		}
	}
	return res
}

func parseInt(s string, size int) (int64, error) {
	val, err := strconv.ParseInt(s, 0, size*8)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q: %w", s, err)
	}
	return val, nil
}

func parseFloat(s string) (float32, error) {
	val, err := strconv.ParseFloat(s, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid float %q: %w", s, err)
	}
	return float32(val), nil
}

func joinInts(vals []int64) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = fmt.Sprintf("%d", v)
	}
	return strings.Join(parts, ",")
}

func printGlobalUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s <command> [options]\n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  read   One-shot read from an area\n")
	fmt.Fprintf(os.Stderr, "  poll   Repeated read with interval\n")
	fmt.Fprintf(os.Stderr, "  write  Write data to an area\n\n")
	fmt.Fprintf(os.Stderr, "Common flags (read/poll/write):\n")
	fmt.Fprintf(os.Stderr, "  -addr string    PLC address (default 127.0.0.1)\n")
	fmt.Fprintf(os.Stderr, "  -rack int       Rack number (default 0)\n")
	fmt.Fprintf(os.Stderr, "  -slot int       Slot number (default 1)\n")
	fmt.Fprintf(os.Stderr, "  -ctype uint     Connection type (default 0)\n")
	fmt.Fprintf(os.Stderr, "  -port int       TCP port (default 102)\n")
	fmt.Fprintf(os.Stderr, "  -area string    Memory area: DB|PE|PA|MK|TM|CT (default DB)\n")
	fmt.Fprintf(os.Stderr, "  -db int         DB number when area=DB (default 1)\n")
	fmt.Fprintf(os.Stderr, "  -start int      Start offset in bytes (default 0)\n")
	fmt.Fprintf(os.Stderr, "  -size int       Byte count to read/write (default 4)\n")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(1)
}

// mathFromBits and mathToBits avoid pulling in the whole math package for Float32 conversions.
func mathFromBits(b uint32) float32 {
	return *(*float32)(unsafe.Pointer(&b))
}

func mathToBits(f float32) uint32 {
	return *(*uint32)(unsafe.Pointer(&f))
}
