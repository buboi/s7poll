# s7poll

Simple CLI, similar to modpoll, for reading/writing Siemens S7 PLC areas using [`github.com/danclive/snap7-go`](https://github.com/danclive/snap7-go).

## Prerequisites

- Go 1.22+
- Native Snap7 library installed and discoverable by the linker (e.g. `libsnap7.so` on Linux). See the upstream snap7 docs for install/build steps.

## Build

```bash
go build ./...
```

Or run without producing a binary:

```bash
go run .
```

## Usage

```
s7poll <command> [flags]

Commands:
  read   One-shot read from an area
  poll   Repeated read with interval
  write  Write data to an area

Common flags:
  -addr string    PLC address (default 127.0.0.1)
  -rack int       Rack number (default 0)
  -slot int       Slot number (default 1)
  -ctype uint     Connection type (default 0)
  -port int       TCP port (default 102)
  -area string    Memory area: DB|PE|PA|MK|TM|CT (default DB)
  -db int         DB number when area=DB (default 1)
  -start int      Start offset in bytes (default 0)
  -size int       Byte count to read/write (default 4)
```

### Examples

Read 4 bytes from DB1, starting at byte 0, output as hex:

```bash
go run . read -addr 192.168.0.10 -area DB -db 1 -start 0 -size 4 -format hex
```

Poll a marker area every second, decoding as int16:

```bash
go run . poll -addr 192.168.0.10 -area MK -start 0 -size 4 -format int16 -interval 1s
```

Write two float32 values to DB3 starting at byte 12:

```bash
go run . write -addr 192.168.0.10 -area DB -db 3 -start 12 -format float32 -values 1.5,2.25
```
