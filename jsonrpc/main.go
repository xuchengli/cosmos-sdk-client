package main

import(
    "encoding/json"
    "fmt"
    "github.com/tendermint/tendermint/types"
    "io"
    "net/http"

    rpcserver "cosmos-client/jsonrpc/server"
    ctypes "github.com/tendermint/tendermint/rpc/core/types"
    rpctypes "github.com/tendermint/tendermint/rpc/jsonrpc/types"
)

func main() {
    config := rpcserver.DefaultConfig()

    mux := http.NewServeMux()
    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        b, err := io.ReadAll(r.Body)
		if err != nil {
			fmt.Printf("解析body失败: %s\n", err.Error())
			return
		}

        var (
			requests  []rpctypes.RPCRequest
			responses []rpctypes.RPCResponse
		)
		if err := json.Unmarshal(b, &requests); err != nil {
			// next, try to unmarshal as a single request
			var request rpctypes.RPCRequest
			if err := json.Unmarshal(b, &request); err != nil {
				fmt.Printf("生成request失败: %s\n", err.Error())
				return
			}
			requests = []rpctypes.RPCRequest{request}
		}

        for _, request := range requests {
            request := request

            // TODO: checkTx

            tx := types.Tx(request.Params)
            result := &ctypes.ResultBroadcastTx{Hash: tx.Hash()}

            responses = append(responses, rpctypes.NewRPCSuccessResponse(request.ID, result))
        }

        if len(responses) > 0 {
			if err := rpcserver.WriteRPCResponseHTTP(w, responses...); err != nil {
                fmt.Errorf("failed to write responses: %s", err)
			}
		}
    })

    listener, err := rpcserver.Listen("tcp://0.0.0.0:26657", config.MaxOpenConnections)
    if err != nil {
        fmt.Errorf("启动监听失败: %s", err)
    }

    if err := rpcserver.Serve(listener, mux, config); err != nil {
        fmt.Errorf("Error serving server: %s", err)
    }
}
