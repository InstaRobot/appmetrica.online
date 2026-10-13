package chstore

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Вставка с ожиданием ack держит соединение до сброса буфера async insert (до 1 с, users.d), поэтому при
// 500 rps (NFR-001) нужно ~500 соединений одновременно. Пул драйвера по умолчанию (10 открытых, 5 простаивающих)
// закрывал и заново открывал соединения на каждую вставку — handshake EOF и 503 под нагрузкой k6 (T-015).
func TestOptions_PoolSizedForIngestLoad(t *testing.T) {
	o := Options{Addr: "localhost:9000", Database: "appmetrica", User: "appmetrica_rw"}.clickhouse()
	require.GreaterOrEqual(t, o.MaxOpenConns, 500, "500 rps × ожидание ack до 1 с")
	require.Equal(t, o.MaxOpenConns, o.MaxIdleConns, "простаивающие соединения не закрываются под нагрузкой")
}
