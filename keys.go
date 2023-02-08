package main

import (
    "cosmos-client/simapp"
    "fmt"
    "github.com/cosmos/cosmos-sdk/crypto/hd"
    "github.com/cosmos/cosmos-sdk/crypto/keyring"
    "github.com/tendermint/tendermint/libs/log"
    "strings"

    simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
    sdk "github.com/cosmos/cosmos-sdk/types"
    dbm "github.com/tendermint/tm-db"
)

func main() {
    app := simapp.NewSimApp(log.NewNopLogger(), dbm.NewMemDB(), nil, true, simtestutil.NewAppOptionsWithFlagHome(simapp.DefaultNodeHome))

    kb, err := keyring.New("sim", keyring.BackendOS, simapp.DefaultNodeHome, strings.NewReader(""), app.AppCodec())
	if err != nil {
		panic(err)
	}

    r, _, err := kb.NewMnemonic("bob", keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
    if err != nil {
        panic(err)
    }
    fmt.Println(r.Name)

    /*
    records, err := kb.List()
    if err != nil {
        panic(err)
    }
    for _, record := range records {
        addr, err := record.GetAddress()
        if err != nil {
            panic(err)
        }

        fmt.Printf("uid: %s, addr: %s\n", record.Name, addr.String())

        armor, err := kb.ExportPrivKeyArmor(record.Name, "passw0rd")
        if err != nil {
            panic(err)
        }

        fmt.Println(armor)
    }
    */
}
