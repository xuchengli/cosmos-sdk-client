package main

import (
    "context"
    "cosmos-client/simapp"
    "fmt"
    "time"

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

func (tc TxClient) preSign(acc *Account) error {
    for {
        accNum, accSeq, err := tc.queryAccount(acc.addr)
        if err != nil {
            return err
        }

        fmt.Println("=================")
        fmt.Printf("account number: %d, sequence: %d\n", accNum, accSeq)
        fmt.Printf("account: %+v\n", *acc)
        fmt.Println("=================")

        if accSeq > acc.accSeq {
            acc.accSeq = accSeq
            acc.accNum = accNum

            return nil
        }
        time.Sleep(time.Second)
    }
}

func (tc TxClient) sendTx(acc1, acc2 *Account, amount int64) error {
    // Create a new TxBuilder.
    txBuilder := tc.app.TxConfig().NewTxBuilder()

    // Define x/bank MsgSend messages:
    msg := banktypes.NewMsgSend(acc1.addr, acc2.addr, sdk.NewCoins(sdk.NewInt64Coin("stake", amount)))

    if err := txBuilder.SetMsgs(msg); err != nil {
        return err
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
        return err
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
            return err
        }
        sigsV2 = append(sigsV2, sigV2)
    }
    if err := txBuilder.SetSignatures(sigsV2...); err != nil {
        return err
    }

    // Generated Protobuf-encoded bytes.
    txBytes, err := tc.app.TxConfig().TxEncoder()(txBuilder.GetTx())
    if err != nil {
        return err
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
        return err
    }

    fmt.Println(grpcRes.TxResponse)

    return nil
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
        addr: ligpAddress,
        priv: ligpPrivKey,
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

    // 转账
    accounts := []*Account{&lixuc, &ligp}
    var acc1, acc2 *Account

    for i := 0; i < 10; i++ {
        if i % 2 == 0 {
            acc1 = accounts[0]
            acc2 = accounts[1]
        } else {
            acc1 = accounts[1]
            acc2 = accounts[0]
        }

        // 查询账户
        // accNum, accSeq, err := txClient.queryAccount(acc1.addr)
        // if err != nil {
        //     panic(err)
        // }
        // acc1.accNum = accNum
        // acc1.accSeq = accSeq
        if err := txClient.preSign(acc1); err != nil {
            panic(err)
        }

        // 转账
        if err := txClient.sendTx(acc1, acc2, int64(i+1)); err != nil {
            panic(err)
        }
    }
}
