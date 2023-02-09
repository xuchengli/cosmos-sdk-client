package main

import (
    "context"
    "cosmos-client/simapp"
    "fmt"
    "sync"

    "google.golang.org/grpc"

    "github.com/cosmos/cosmos-sdk/crypto"
    "github.com/tendermint/tendermint/libs/log"
    "github.com/cosmos/cosmos-sdk/types/tx"
    "github.com/cosmos/cosmos-sdk/types/tx/signing"

    txclient "github.com/cosmos/cosmos-sdk/client/tx"
    cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
    simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
    sdk "github.com/cosmos/cosmos-sdk/types"
    xauthsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
    authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
    banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
    dbm "github.com/tendermint/tm-db"
)

const (
    lixucArmorPrivKey string = `-----BEGIN TENDERMINT PRIVATE KEY-----
kdf: bcrypt
salt: 209F390BD6E0653D9AC3BED85E4499EE
type: secp256k1

GGn020H4+UZhfyqOrvU6lV3KW8UQo3SuB2HxMO2rnzo16PBWsDpX9mcaV+aEThr2
iPZk/nSYn/Ml4aVY6XuWvFfwJAHbemRwbeAxZHM=
=+2hb
-----END TENDERMINT PRIVATE KEY-----`

    ligpArmorPrivKey string = `-----BEGIN TENDERMINT PRIVATE KEY-----
salt: C1F283351E6F6A9C07A748764B07D4F2
type: secp256k1
kdf: bcrypt

C/KqjXGU/3X+ZsWfRhqS400/2+l6NN0GOzoPkYJuFTs9POnDXhwhRZmt1/UOj9na
MKL18PP8LCPjVPevYJckNZOfhAstrGbbHNQmfSw=
=KLRC
-----END TENDERMINT PRIVATE KEY-----`

    bobArmorPrivKey string = `-----BEGIN TENDERMINT PRIVATE KEY-----
kdf: bcrypt
salt: 09FA96C26AFD460C1DE1301AD1FED824
type: secp256k1

ySjPj2d6R/n0B7wVoI0Uz3uUsNEbgsMReNCKpvszZ0tcV+IDl2fd9gua3kURxdDh
CkLY8VZYztPaxuIslQVPh8FVxfJOGgvRRLJE4Qs=
=Nhhh
-----END TENDERMINT PRIVATE KEY-----`
)

// TxClient 交易
type TxClient struct {
    ctx           context.Context
    app           *simapp.SimApp
    authClient    authtypes.QueryClient
    serviceClient tx.ServiceClient
}

// Account 账户
type Account struct {
    uid    string
    addr   sdk.AccAddress
    priv   cryptotypes.PrivKey
    accNum uint64
    accSeq uint64
}

func (tc TxClient) queryAccount(addr sdk.AccAddress) (uint64, uint64, error) {
    authRes, err := tc.authClient.Account(tc.ctx, &authtypes.QueryAccountRequest{Address: addr.String()})
    if err != nil {
        return 0, 0, err
    }

    var acc authtypes.AccountI
	if err := tc.app.InterfaceRegistry().UnpackAny(authRes.Account, &acc); err != nil {
		return 0, 0, err
	}

    return acc.GetAccountNumber(), acc.GetSequence(), nil
}

func (tc TxClient) sendTx(acc1, acc2 *Account, amount int64) (uint32, string, string, error) {
    // Create a new TxBuilder.
    txBuilder := tc.app.TxConfig().NewTxBuilder()

    // Define x/bank MsgSend messages:
    msg := banktypes.NewMsgSend(acc1.addr, acc2.addr, sdk.NewCoins(sdk.NewInt64Coin("stake", amount)))

    if err := txBuilder.SetMsgs(msg); err != nil {
        return 1, "", "", err
    }
    txBuilder.SetGasLimit(200000)

    privs := []cryptotypes.PrivKey{acc1.priv}
    accNums:= []uint64{acc1.accNum}
    accSeqs:= []uint64{acc1.accSeq}

    // First round: we gather all the signer infos. We use the "set empty
    // signature" hack to do that.
    var sigsV2 []signing.SignatureV2
    for i, priv := range privs {
        sigV2 := signing.SignatureV2{
            PubKey: priv.PubKey(),
            Data: &signing.SingleSignatureData{
                SignMode:  tc.app.TxConfig().SignModeHandler().DefaultMode(),
                Signature: nil,
            },
            Sequence: accSeqs[i],
        }
        sigsV2 = append(sigsV2, sigV2)
    }
    if err := txBuilder.SetSignatures(sigsV2...); err != nil {
        return 1, "", "", err
    }

    // Second round: all signer infos are set, so each signer can sign.
    sigsV2 = []signing.SignatureV2{}
    for i, priv := range privs {
        signerData := xauthsigning.SignerData{
            ChainID:       "test-chain",
            AccountNumber: accNums[i],
            Sequence:      accSeqs[i],
        }
        sigV2, err := txclient.SignWithPrivKey(
            tc.app.TxConfig().SignModeHandler().DefaultMode(), signerData,
            txBuilder, priv, tc.app.TxConfig(), accSeqs[i])
        if err != nil {
            return 1, "", "", err
        }
        sigsV2 = append(sigsV2, sigV2)
    }
    if err := txBuilder.SetSignatures(sigsV2...); err != nil {
        return 1, "", "", err
    }

    // Generated Protobuf-encoded bytes.
    txBytes, err := tc.app.TxConfig().TxEncoder()(txBuilder.GetTx())
    if err != nil {
        return 1, "", "", err
    }

    // Generate a JSON string.
    /*
    txJSONBytes, err := tc.app.TxConfig().TxJSONEncoder()(txBuilder.GetTx())
    if err != nil {
        return err
    }
    txJSON := string(txJSONBytes)
    fmt.Println(txJSON)
    */

    grpcRes, err := tc.serviceClient.BroadcastTx(
        tc.ctx,
        &tx.BroadcastTxRequest{
            Mode:    tx.BroadcastMode_BROADCAST_MODE_SYNC,
            TxBytes: txBytes,
        },
    )
    if err != nil {
        return 1, "", "", err
    }

    return grpcRes.TxResponse.Code, grpcRes.TxResponse.TxHash, grpcRes.TxResponse.RawLog, nil
}

func (tc TxClient) mustSendTx(acc1, acc2 *Account, amount int64) (uint32, string, string, error) {
    for {
        code, hash, log, err := tc.sendTx(acc1, acc2, amount)
        if err != nil {
            return 1, "", "", err
        }
        if code == 20 {
            continue
        }
        return code, hash, log, nil
    }
}

func main() {
    app := simapp.NewSimApp(log.NewNopLogger(), dbm.NewMemDB(), nil, true, simtestutil.NewAppOptionsWithFlagHome(simapp.DefaultNodeHome))

    // 准备账户
    lixucAddress, err := sdk.AccAddressFromBech32("cosmos1dxfeuswss2mrgsq7lv36lwk3272g99f07anqff")
    if err != nil {
        panic(err)
    }
    lixucPrivKey, _, err := crypto.UnarmorDecryptPrivKey(lixucArmorPrivKey, "passw0rd")
    if err != nil {
        panic(err)
    }
    lixuc := Account{
        uid:  "lixuc",
        addr: lixucAddress,
        priv: lixucPrivKey,
    }

    ligpAddress, err := sdk.AccAddressFromBech32("cosmos1vqhcxnhkmcm4ptphy75lqvh0daysf8eyuwccnv")
    if err != nil {
        panic(err)
    }
    ligpPrivKey, _, err := crypto.UnarmorDecryptPrivKey(ligpArmorPrivKey, "passw0rd")
    if err != nil {
        panic(err)
    }
    ligp := Account{
        uid:  "ligp",
        addr: ligpAddress,
        priv: ligpPrivKey,
    }

    bobAddress, err := sdk.AccAddressFromBech32("cosmos12llzcc3xz5mn3cqdppte9729546md4flsf6c7q")
    if err != nil {
        panic(err)
    }
    bobPrivKey, _, err := crypto.UnarmorDecryptPrivKey(bobArmorPrivKey, "passw0rd")
    if err != nil {
        panic(err)
    }
    bob := Account{
        uid:  "bob",
        addr: bobAddress,
        priv: bobPrivKey,
    }

    // Create a connection to the gRPC server.
    grpcConn, err := grpc.Dial("127.0.0.1:9090", grpc.WithInsecure())
    if err != nil {
        panic(err)
    }
    defer grpcConn.Close()

    // 准备交易
    txClient := TxClient{
        ctx:           context.Background(),
        app:           app,
        authClient:    authtypes.NewQueryClient(grpcConn),
        serviceClient: tx.NewServiceClient(grpcConn),
    }

    accounts := []Account{lixuc, ligp}

    var wg sync.WaitGroup
    wg.Add(len(accounts))

    for i := 0; i < len(accounts); i++ {
        go func(acc Account, deferFunc func()) {
            defer deferFunc()

            // 查询账户
            accNum, accSeq, err := txClient.queryAccount(acc.addr)
            if err != nil {
                panic(err)
            }
            acc.accNum = accNum
            acc.accSeq = accSeq

            // 转账
            for i := 0; i < 3000; i++ {
                code, hash, log, err := txClient.mustSendTx(&acc, &bob, 1)
                if err != nil {
                    panic(err)
                }

                fmt.Printf("account: %s, code: %d, txhash: %s, rawlog: %s\n", acc.uid, code, hash, log)

                acc.accSeq = acc.accSeq + 1
            }

        }(accounts[i], wg.Done)
    }
    wg.Wait()
}
