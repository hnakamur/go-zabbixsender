package zabbix

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

// https://www.zabbix.com/documentation/current/en/manual/appendix/protocols/zabbix_sender
// https://www.zabbix.com/documentation/current/en/manual/appendix/items/trapper

const defaultZabbixServerPort = 10051

// nonLargePacketSizeLimit is the limit of the size of "non-large" packet.
// This pacakge does not support large packets.
// https://www.zabbix.com/documentation/current/en/manual/appendix/protocols/header_datalen
const nonLargePacketSizeLimit = 1024 * 1024 * 1024 // 1GiB

var ErrRequestPacketSizeLimitExeeded = errors.New("request packet size limit exceeded")

const protocol = "ZBXD"
const protocolFlagZabbixCommunications = '\x01'
const dataLenOffset = len(protocol) + 1
const dataLenLen = 4
const reservedLen = 4
const headerLen = dataLenOffset + dataLenLen + reservedLen

const requestType = "sender data"

type Sender struct {
	ServerAddress string
	Timeout       time.Duration
}

type request struct {
	Request string        `json:"request"`
	Data    []TrapperData `json:"data"`
}

type TrapperData struct {
	Host  string `json:"host"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Response struct {
	Response     string  `json:"response"`
	Info         string  `json:"info"`
	Processed    int     `json:"-"`
	Failed       int     `json:"-"`
	Total        int     `json:"-"`
	SecondsSpent float64 `json:"-"`
}

func (s *Sender) Send(data []TrapperData) (*Response, error) {
	deadline := time.Now().Add(s.Timeout)
	reqPacket, err := buildRequestPacket(data)
	if err != nil {
		return nil, err
	}

	addr := addDefaultPortToAddressIfNeeded(s.ServerAddress)
	conn, err := net.DialTimeout("tcp", addr, s.Timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := conn.SetWriteDeadline(deadline); err != nil {
		return nil, err
	}

	n, err := conn.Write(reqPacket)
	if err != nil {
		return nil, fmt.Errorf("send request packet: %s", err)
	}
	if n < len(reqPacket) {
		return nil, errors.New("short write for sending request packet")
	}

	if err := conn.SetReadDeadline(deadline); err != nil {
		return nil, err
	}
	return parseResponse(conn)
}

func addDefaultPortToAddressIfNeeded(addr string) string {
	_, _, err := net.SplitHostPort(addr)
	if err != nil {
		return net.JoinHostPort(addr, strconv.Itoa(defaultZabbixServerPort))
	}
	return addr
}

func buildRequestPacket(data []TrapperData) ([]byte, error) {
	var b bytes.Buffer
	if _, err := b.WriteString(protocol); err != nil {
		return nil, err
	}
	if err := b.WriteByte(protocolFlagZabbixCommunications); err != nil {
		return nil, err
	}

	const tmpDataLen = 0
	if err := binary.Write(&b, binary.LittleEndian, uint32(tmpDataLen)); err != nil {
		return nil, err
	}

	const reserved = 0
	if err := binary.Write(&b, binary.LittleEndian, uint32(reserved)); err != nil {
		return nil, err
	}

	req := request{
		Request: requestType,
		Data:    data,
	}
	enc := json.NewEncoder(&b)
	if err := enc.Encode(req); err != nil {
		return nil, err
	}

	packet := b.Bytes()
	packetLen := len(packet)
	if packetLen > nonLargePacketSizeLimit {
		return nil, ErrRequestPacketSizeLimitExeeded
	}

	dataLen := uint32(packetLen - headerLen)
	binary.LittleEndian.PutUint32(packet[dataLenOffset:dataLenOffset+dataLenLen], dataLen)

	return packet, nil
}

func parseResponse(r io.Reader) (*Response, error) {
	var headerBuf [headerLen]byte
	if _, err := io.ReadFull(r, headerBuf[:]); err != nil {
		return nil, fmt.Errorf("read response header: %s", err)
	}

	if !bytes.HasPrefix(headerBuf[:], []byte(protocol)) {
		return nil, errors.New("unexpected response protocol")
	}
	if headerBuf[len(protocol)] != protocolFlagZabbixCommunications {
		return nil, errors.New("unsupported response protocol flag")
	}

	dataLen := binary.LittleEndian.Uint32(headerBuf[dataLenOffset : dataLenOffset+dataLenLen])
	dataBuf := make([]byte, dataLen)
	if _, err := io.ReadFull(r, dataBuf); err != nil {
		return nil, fmt.Errorf("read response data: %s", err)
	}

	var resp Response
	if err := json.Unmarshal(dataBuf, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %s", err)
	}

	n, err := fmt.Sscanf(resp.Info, "processed: %d; failed: %d; total: %d; seconds spent: %f",
		&resp.Processed, &resp.Failed, &resp.Total, &resp.SecondsSpent)
	if err != nil {
		return nil, fmt.Errorf("parse response info: %s", err)
	}
	if n != 4 {
		return nil, fmt.Errorf("unexpected scan result count in parsing response: %d", n)
	}

	return &resp, nil
}

func (r *Response) IsSucccess() bool {
	return r.Response == "success" && r.Failed == 0
}
