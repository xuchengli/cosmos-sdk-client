package main

import (
    "context"
    "cosmos-client/simapp"
    "fmt"
    "time"

    "google.golang.org/grpc"
    "github.com/goombaio/namegenerator"

    "github.com/cosmos/cosmos-sdk/crypto"
    "github.com/cosmos/cosmos-sdk/crypto/hd"
    "github.com/cosmos/cosmos-sdk/crypto/keyring"
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
    genesisRawAddress string = "cosmos15e69645gtzwl5pl9yj3us0wvzp4n6zg9uzhru7"
    genesisArmorPrivKey string = `-----BEGIN TENDERMINT PRIVATE KEY-----
type: secp256k1
kdf: bcrypt
salt: 516ACBC5A67598787E4B0833B6FBD08F

II8jjQ6pp9N0bsxJxiy0wCCTThpdkkCKms9Fl5/yE21GlqZn5m2D3hVhwlU6pJn5
6Kc5NZCi+EW/FnV59nv8L3TJ2UHQO0JI4x/6rC4=
=3Me5
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
        // mempool 满了等待
        if code == 20 {
            continue
        }
        return code, hash, log, nil
    }
}

func (tc TxClient) generateAccount(genesisAccount Account) (*Account, error) {
    seed := time.Now().UTC().UnixNano()
    nameGenerator := namegenerator.NewNameGenerator(seed)
    name := nameGenerator.Generate()

    kr := keyring.NewInMemory(tc.app.AppCodec())

    record, _, err := kr.NewMnemonic(name, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
    if err != nil {
        return nil, err
    }

    addr, err := record.GetAddress()
    if err != nil {
        return nil, err
    }

    armorPrivKey, err := kr.ExportPrivKeyArmor(record.Name, "passw0rd")
    if err != nil {
        return nil, err
    }
    privKey, _, err := crypto.UnarmorDecryptPrivKey(armorPrivKey, "passw0rd")
    if err != nil {
        return nil, err
    }

    account := Account{
        uid:  name,
        addr: addr,
        priv: privKey,
    }

    // 通过创世账户转账给新账户, 新账户进行上链
    code, hash, log, err := tc.mustSendTx(&genesisAccount, &account, 1000)
    if err != nil {
        return nil, err
    }
    fmt.Printf("激活账户: %s, 地址: %s, code: %d, txhash: %s, rawlog: %s\n", name, addr.String(), code, hash, log)

    return &account, nil
}

func main() {
    app := simapp.NewSimApp(log.NewNopLogger(), dbm.NewMemDB(), nil, true, simtestutil.NewAppOptionsWithFlagHome(simapp.DefaultNodeHome))

    // 创世账户
    genesisAddress, err := sdk.AccAddressFromBech32(genesisRawAddress)
    if err != nil {
        panic(err)
    }
    genesisPrivKey, _, err := crypto.UnarmorDecryptPrivKey(genesisArmorPrivKey, "passw0rd")
    if err != nil {
        panic(err)
    }
    genesis := Account{
        addr: genesisAddress,
        priv: genesisPrivKey,
    }

    // gRPC连接
    grpcConn, err := grpc.Dial("127.0.0.1:9090", grpc.WithInsecure())
    if err != nil {
        panic(err)
    }
    defer grpcConn.Close()

    // 客户端
    txClient := TxClient{
        ctx:           context.Background(),
        app:           app,
        authClient:    authtypes.NewQueryClient(grpcConn),
        serviceClient: tx.NewServiceClient(grpcConn),
    }

    // 生成一批账户
    accounts := []*Account{}

    // 查询创世账户的编号和签名序列号
    accNum, accSeq, err := txClient.queryAccount(genesis.addr)
    if err != nil {
        panic(err)
    }
    genesis.accNum = accNum
    genesis.accSeq = accSeq

    for i := 0; i < 5; i++ {
        account, err := txClient.generateAccount(genesis)
        if err != nil {
            panic(err)
        }
        genesis.accSeq = genesis.accSeq + 1

        accounts = append(accounts, account)
    }

    time.Sleep(time.Second * 5)

    // 设置所有账户的编号和签名序列号
    for _, acc := range accounts {
        // 查询账户
        accNum, accSeq, err := txClient.queryAccount(acc.addr)
        if err != nil {
            panic(err)
        }
        acc.accNum = accNum
        acc.accSeq = accSeq
    }

    // 轮流转账
    for {
        func(from, to *Account) {
            // 转账
            for i := 0; i < 1000; i++ {
                code, hash, log, err := txClient.mustSendTx(from, to, 1)
                if err != nil {
                    panic(err)
                }

                fmt.Printf("account: %s, code: %d, txhash: %s, rawlog: %s\n", from.uid, code, hash, log)

                from.accSeq = from.accSeq + 1
            }
        }(accounts[0], accounts[1])

        accounts = append(accounts[1:], accounts[0])
    }
}
