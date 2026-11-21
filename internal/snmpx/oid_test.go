package snmpx

import (
	"reflect"
	"testing"
)

func TestCompareOID(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.3.6", "1.3.6", 0},
		{"1.3.6", "1.3.6.1", -1},
		{"1.3.7", "1.3.6.1", 1},
		{"1.3.6.1.2", "1.3.6.1.10", -1}, // 数値比較（文字列比較なら逆になる）
	}
	for _, c := range cases {
		if got := CompareOID(MustOID(c.a), MustOID(c.b)); got != c.want {
			t.Errorf("Compare(%s,%s)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestPortListBits(t *testing.T) {
	// 0x80=ポート1, 0x01=ポート8, 2バイト目0x40=ポート10
	got := PortListBits([]byte{0x81, 0x40})
	if !reflect.DeepEqual(got, []int{1, 8, 10}) {
		t.Fatalf("got %v", got)
	}
	// 52ポート相当（7バイト）でもポート52を復号できる
	b := MakePortList([]int{52}, 7)
	if got := PortListBits(b); !reflect.DeepEqual(got, []int{52}) {
		t.Fatalf("52: got %v", got)
	}
	if PortListBits(nil) != nil {
		t.Fatal("空入力はnil")
	}
}

func TestDecodeInetIndex(t *testing.T) {
	s, n := DecodeInetIndex([]uint32{1, 4, 239, 1, 2, 3, 99})
	if s != "239.1.2.3" || n != 6 {
		t.Fatalf("got %s %d", s, n)
	}
	if _, n := DecodeInetIndex([]uint32{1, 4, 239}); n != 0 {
		t.Fatal("長さ不足は0")
	}
}

func TestSuffix(t *testing.T) {
	idx, ok := Suffix(".1.3.6.1.2.1.2.2.1.8.12", "1.3.6.1.2.1.2.2.1.8")
	if !ok || !reflect.DeepEqual(idx, []uint32{12}) {
		t.Fatalf("got %v %v", idx, ok)
	}
	if _, ok := Suffix("1.3.6.1.2.1.2.2.1.80.1", "1.3.6.1.2.1.2.2.1.8"); ok {
		t.Fatal("接頭辞の部分一致を誤判定")
	}
}
