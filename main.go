// tinygo build -target=pico  -size=short -o SignalGenerator.uf2 .
// tinygo flash -target=pico  -size=short -monitor .
// tinygo build -target=pico2 -size=short -o SignalGenerator2.uf2 .
// tinygo flash -target=pico2 -size=short -monitor .

// 信号発生器（Signal Generator）
// tinygoで作成したRaspiPicoから微弱電波を発生させる信号発生器

// Package main は、TinyGoを使用してRaspberry Pi Picoから微弱電波を発生させる信号発生器のメインパッケージです[cite: 1]。
package main

import (
	"fmt"
	"machine"
	"strconv"
	"strings"
	"time"

	"github.com/triring/transmitter" // githubで公開しているパッケージをインポートする。
)

// pinToPWM は、Raspberry Pi PicoのGPIOピンとPWMチャンネルの対応表です[cite: 1]。
var pinToPWM = map[machine.Pin]transmitter.PWM{
	machine.GPIO0:  machine.PWM0,
	machine.GPIO1:  machine.PWM0,
	machine.GPIO2:  machine.PWM1,
	machine.GPIO3:  machine.PWM1,
	machine.GPIO4:  machine.PWM2,
	machine.GPIO5:  machine.PWM2,
	machine.GPIO6:  machine.PWM3,
	machine.GPIO7:  machine.PWM3,
	machine.GPIO8:  machine.PWM4,
	machine.GPIO9:  machine.PWM4,
	machine.GPIO10: machine.PWM5,
	machine.GPIO11: machine.PWM5,
	machine.GPIO12: machine.PWM6,
	machine.GPIO13: machine.PWM6,
	machine.GPIO14: machine.PWM7,
	machine.GPIO15: machine.PWM7,
	machine.GPIO16: machine.PWM0,
	machine.GPIO17: machine.PWM0,
	machine.GPIO18: machine.PWM1,
	machine.GPIO19: machine.PWM1,
	machine.GPIO20: machine.PWM2,
	machine.GPIO21: machine.PWM2,
	machine.GPIO22: machine.PWM3,
	machine.GPIO23: machine.PWM3,
	machine.GPIO24: machine.PWM4,
	machine.GPIO25: machine.PWM4,
	machine.GPIO26: machine.PWM5,
	machine.GPIO27: machine.PWM5,
	machine.GPIO28: machine.PWM6,
	machine.GPIO29: machine.PWM6,
}

var (
	txFlag   bool   = false
	txFreq   uint64 = 999 * machine.KHz
	toneFreq int    = 880
)

var (
	TxFreqMin   uint64 = 0
	TxFreqMax   uint64 = 25 * machine.MHz
	ToneFreqMin uint64 = 0
	ToneFreqMax uint64 = 320 * machine.KHz
)

// AM放送の周波数帯（526.5～1606.5KHz）に対応するPWM設定用の定数
// これに適合する誤差の少ない周波数を生成できるPWMの設定は、以下の通り。
// これ以外の周波数を使用する場合は、周波数の誤差は大きくなるが、以下のように出力したい周波数をHzで設定すればよい。
const (
	AM0612KHz uint64 = 612 * machine.KHz
	AM0684KHz uint64 = 684 * machine.KHz
	AM0693KHz uint64 = 693 * machine.KHz
	AM0738KHz uint64 = 738 * machine.KHz
	AM0774KHz uint64 = 774 * machine.KHz
	AM0819KHz uint64 = 819 * machine.KHz
	AM0999KHz uint64 = 999 * machine.KHz
	AM1224KHz uint64 = 1224 * machine.KHz
	AM1269KHz uint64 = 1269 * machine.KHz
	AM1287KHz uint64 = 1287 * machine.KHz
	AM1368KHz uint64 = 1368 * machine.KHz
	AM1548KHz uint64 = 1548 * machine.KHz
)

// 逓倍波を利用してFM放送の周波数帯に適合させるためのPWM設定用の定数
// 日本のFM放送の周波数帯は、76.1～94.9MHzである。
// 世界のFM放送の周波数帯は、87.5～108.0 MHz)が一般的である。
// これらの周波数帯の波長をRaspberry pi picoのPWMで生成する不可能である。
// そこで、逓倍波で、これらに適合する周波数となるPWMの設定を探した。
// その結果は、以下のとおりである。
const (
	Freq2MHz    uint64 = 2 * machine.MHz
	Freq4MHz    uint64 = 4 * machine.MHz
	Freq5_20MHz uint64 = 5 * machine.MHz
	Freq8MHz    uint64 = 8 * machine.MHz
	Freq10MHz   uint64 = 10 * machine.MHz
	Freq16MHz   uint64 = 16 * machine.MHz
	Freq20MHz   uint64 = 20 * machine.MHz
	Freq25MHz   uint64 = 25 * machine.MHz
)

// HelpText は、コンソール上で表示するコマンド一覧のメッセージ定義です[cite: 1]。
var HelpText = [...]string{
	"Command list",
	"  H             :(Help)       コマンドの使用方法を表示する。",
	"  F [tx_freq]   :(Frequency)  送信周波数を設定する。",
	"  F             :(Frequency)  送信周波数を確認する。",
	"  T [tone_freq] :(Tone)       発信音の周波数を設定する。",
	"  T             :(Tone)       発信音の周波数を確認する。",
	"  S             :(Start/Stop) 信号の送信と停止を行う。",
	"  Q             :(Quit)       終了する。",
}

// initTransmitter は、指定されたPWMチャンネルとGPIOピンを使用して送信機を初期化します[cite: 1]。
// 引数 pwm: 使用するPWMチャンネル、antPin: アンテナ線に接続するGPIOピン
func initTransmitter(pwm transmitter.PWM, antPin machine.Pin) (transmitter.Transmitter, error) {
	wave, err := transmitter.New(pwm, antPin)
	if err != nil {
		return wave, err
	}
	return wave, nil
}

// txWorker は、バックグラウンド（Goroutine）で指定された周波数とトーンで信号を送出するワーカー関数です[cite: 1]。
// doneChan からの終了通知を受け取るか、コマンドで停止されるまで動作を継続します。
func txWorker(doneChan <-chan struct{}, tx transmitter.Transmitter, txFreq uint64, toneFreq int) {
	// 発信音の設定
	interval := time.Duration(1000) * time.Millisecond / time.Duration(toneFreq)
	// 送信周波数の設定
	tx.SetFrequency(txFreq)
	for {
		select {
		case <-doneChan:
			tx.Stop() // 出力を停止する。
			return
		default:
			// 定常処理 設定した発信音になるように断続的に送信波を出力する。
			for i := 0; i < 100; i++ {
				tx.SetDutyRatio(2)
				time.Sleep(interval)
				tx.Stop()
				time.Sleep(interval)
			}
		}
	}
}

// trimLastChar は、指定された文字列の最後のルーン（文字）を削除して返します。
func trimLastChar(s string) string {
	if s == "" {
		return ""
	}
	// 文字列をルーンのスライスに変換する
	runes := []rune(s)
	// スライスの最後の要素を除外して、新しい文字列として返す。
	return string(runes[:len(runes)-1])
}

// inRange は、指定された値が最小値から最大値の範囲内にあるかを判定します。
// 範囲内の場合はtrueを返します。
func inRange(min, value, max uint64) bool {
	return value >= min && value <= max
}

// getCommand は、シリアルコンソールからユーザー入力を1行分受け取り、
// バックスペースなどの制御文字を処理した上で入力文字列を返します。
func getCommand() string {
	enterFlag := false // 改行コードのチェック用フラグ
	var readbuffer string
	for { // キー入力待ち
		// PCからの受信データをチェック
		if machine.Serial.Buffered() > 0 {
			c, err := machine.Serial.ReadByte()
			if err == nil {
				if c < 32 {
					switch c {
					case '\r', '\n':
						enterFlag = true
					case '\b':
						if len(readbuffer) > 0 { // バックスペースで、最後尾の１文字を削除
							machine.Serial.WriteByte('\b') // 表示部分の最後の1文字を消去
							machine.Serial.WriteByte(' ')
							machine.Serial.WriteByte('\b')
							readbuffer = trimLastChar(readbuffer) // すでに取り込んでいる文字列データの最後の1文字を消去
						}
					default:
						// println(c)	  -- >  BS=8, Enter=13
						// Convert nonprintable control characters to
						// ^A, ^B, etc.
						machine.Serial.WriteByte('^')
						machine.Serial.WriteByte(c + '@')
					}
				} else if c >= 127 {
					// Anything equal or above ASCII 127, print ^?.
					machine.Serial.WriteByte('^')
					machine.Serial.WriteByte('?')
				} else {
					// Echo the printable character back to the
					// host computer.
					machine.Serial.WriteByte(c)
					// 読み込んだ文字をエコーバックし、文字列バッファーに保存する。
					readbuffer += string(c)
				}
			}
		}
		// This assumes that the input is coming from a keyboard
		// so checking 120 times per second is sufficient. But if
		// the data comes from another processor, the port can
		// theoretically receive as much as 11000 bytes/second
		// (115200 baud). This delay can be removed and the
		// Serial.Read() method can be used to retrieve
		// multiple bytes from the receive buffer for each
		// iteration.
		if enterFlag {
			// 改行コードを検出したら、ループを抜け、取り込んだ文字列を返す。
			fmt.Printf("\n")
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return readbuffer
}

func main() {
	txFlag = false
	txFreq = 999 * machine.KHz
	toneFreq = 880
	time.Sleep(1000 * time.Millisecond)
	fmt.Printf("Signal Generator version 1.0\r\n")
	for _, text := range HelpText {
		fmt.Printf("%s\r\n", text)
	}

	// 出力アンテナに設定するGPIOピンに対応するpwmのチャンネルを取得する。
	pwm := pinToPWM[machine.GPIO15]
	// 微弱電波を出力するGPIOピンを指定して、送信機を設定する。
	tx, err := initTransmitter(pwm, machine.GPIO15)
	if err != nil {
		fmt.Printf("failed to configure PWM\r\n")
		return
	}

	// メインループ：外部からの制御コマンドの受付と処理を繰り返す
	// コマンドの受付部の処理プロトタイプ
	doneChan := make(chan struct{})
	execStatus := true
	for execStatus {
		fmt.Printf("> ")
		readbuffer := getCommand() // コンソールから文字列を受け取る。
		// コマンドの解析を開始
		if len(readbuffer) > 0 {
			line := strings.ReplaceAll(readbuffer, "\t", " ")
			line = strings.ReplaceAll(line, ",", " ")
			line = strings.ToUpper(line)
			line = strings.Trim(line, " \n\r")

			elements := strings.Fields(line)
			if len(elements) == 0 {
				continue
			}

			firstWord := elements[0]
			// 先頭文字のコマンドを受けて、それぞれの処理を行う。
			switch firstWord {
			case "H": //	ヘルプの表示(help)
				if len(elements) == 1 {
					for _, text := range HelpText {
						fmt.Printf("%s\n", text)
					}
				}
			case "F": //	出力周波数の設定と参照
				if len(elements) == 1 {
					fmt.Printf("TX Frequency: %d KHz\n", txFreq/1000)
				} else if len(elements) > 1 {
					//	数値変換
					val, err := strconv.ParseInt(strings.Trim(elements[1], " \n\r"), 0, 64)
					if err == nil { //	文字列=>数値変換にエラーがなければ、次のステップへ
						if inRange(TxFreqMin, uint64(val), TxFreqMax) {
							txFreq = uint64(val) * 1000
							fmt.Printf("TX Frequency: %d KHz\n", txFreq/1000)
						} else {
							fmt.Printf("TX Frequency setting error %v\n", elements)
						}
					}
				}
			case "T": //	音程の設定と参照
				if len(elements) == 1 {
					fmt.Printf("Tone Frequency: %d Hz\n", toneFreq)
				} else if len(elements) > 1 {
					//	数値変換
					val, err := strconv.ParseInt(strings.Trim(elements[1], " \n\r"), 0, 64)
					if err == nil { //	文字列=>数値変換にエラーがなければ、次のステップへ
						if inRange(ToneFreqMin, uint64(val), ToneFreqMax) {
							toneFreq = int(val)
							fmt.Printf("Tone Frequency: %d Hz\n", uint64(val))
						} else {
							fmt.Printf("Tone Frequency setting error %v\n", elements)
						}
					}
				}
			case "S": //	信号の送信と停止の切替
				switch txFlag {
				case false:
					doneChan = make(chan struct{})
					// 送信機制御用のゴルーチンをバックグラウンドで起動
					// メイン処理とは非同期で 電波 の送出監視・実行が行われる
					go txWorker(doneChan, tx, txFreq, toneFreq)
					txFlag = true
					fmt.Printf("Send signal\tTX Frequency: %d KHz, Tone Frequency: %dHz\n", txFreq/1000, toneFreq)
				case true:
					close(doneChan)
					txFlag = false
					fmt.Println("Stop signal")
					tx.Stop()
				}
			case "Q":
				txFlag = false
				fmt.Println("Stop signal")
				tx.Stop()
				execStatus = false // プログラムを終了する。			}
			}
		}
	}

	fmt.Printf("program terminated !\n")
	select {
	case _, ok := <-doneChan:
		if ok {
			close(doneChan)
		}
	default:
		// 既にクローズされている場合などの考慮
	}

	for {
		time.Sleep(time.Millisecond * 5000)
	}
}
