package order

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	sdkstore "github.com/cosmos/cosmos-sdk/store"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/bank"

	abci "github.com/tendermint/tendermint/abci/types"
	bc "github.com/tendermint/tendermint/blockchain"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/libs/db"
	"github.com/tendermint/tendermint/libs/log"
	tmtypes "github.com/tendermint/tendermint/types"

	"github.com/BiJie/BinanceChain/common"
	"github.com/BiJie/BinanceChain/common/types"
	me "github.com/BiJie/BinanceChain/plugins/dex/matcheng"
	dextypes "github.com/BiJie/BinanceChain/plugins/dex/types"
	"github.com/BiJie/BinanceChain/plugins/tokens"
	"github.com/BiJie/BinanceChain/wire"
)

func MakeCodec() *wire.Codec {
	var cdc = wire.NewCodec()

	wire.RegisterCrypto(cdc) // Register crypto.
	bank.RegisterWire(cdc)
	sdk.RegisterWire(cdc) // Register Msgs
	tokens.RegisterWire(cdc)
	types.RegisterWire(cdc)
	cdc.RegisterConcrete(NewOrderMsg{}, "dex/NewOrder", nil)
	cdc.RegisterConcrete(CancelOrderMsg{}, "dex/CancelOrder", nil)

	cdc.RegisterConcrete(OrderBookSnapshot{}, "dex/OrderBookSnapshot", nil)
	cdc.RegisterConcrete(ActiveOrders{}, "dex/ActiveOrders", nil)

	return cdc
}

func MakeKeeper(cdc *wire.Codec) *Keeper {
	accountMapper := auth.NewAccountMapper(cdc, common.AccountStoreKey, types.ProtoAppAccount)
	coinKeeper := bank.NewKeeper(accountMapper)
	codespacer := sdk.NewCodespacer()
	keeper, _ := NewKeeper(common.DexStoreKey, coinKeeper, codespacer.RegisterNext(dextypes.DefaultCodespace), 2, cdc)
	return keeper
}

func MakeCMS() sdk.CacheMultiStore {
	memDB := db.NewMemDB()
	ms := sdkstore.NewCommitMultiStore(memDB)
	ms.MountStoreWithDB(common.DexStoreKey, sdk.StoreTypeIAVL, nil)
	ms.LoadLatestVersion()
	cms := ms.CacheMultiStore()
	return cms
}
func TestKeeper_MarkBreatheBlock(t *testing.T) {
	assert := assert.New(t)
	cdc := MakeCodec()
	keeper := MakeKeeper(cdc)
	cms := MakeCMS()
	logger := log.NewTMLogger(os.Stdout)
	ctx := sdk.NewContext(cms, abci.Header{}, true, logger)
	tt, _ := time.Parse(time.RFC3339, "2018-01-02T15:04:05Z")
	ts := tt.UnixNano() / 1000000
	keeper.MarkBreatheBlock(42, ts, ctx)
	kvstore := ctx.KVStore(common.DexStoreKey)
	h := keeper.GetBreatheBlockHeight(tt, kvstore, 10)
	assert.Equal(int64(42), h)
	tt.AddDate(0, 0, 9)
	h = keeper.GetBreatheBlockHeight(tt, kvstore, 10)
	assert.Equal(int64(42), h)
	tt, _ = time.Parse(time.RFC3339, "2018-01-03T15:04:05Z")
	ts = tt.UnixNano() / 1000000
	keeper.MarkBreatheBlock(43, ts, ctx)
	h = keeper.GetBreatheBlockHeight(tt, kvstore, 10)
	assert.Equal(int64(43), h)
	tt.AddDate(0, 0, 9)
	h = keeper.GetBreatheBlockHeight(tt, kvstore, 10)
	assert.Equal(int64(43), h)
}

func Test_compressAndSave(t *testing.T) {
	assert := assert.New(t)
	cdc := MakeCodec()
	//keeper := MakeKeeper(cdc)
	cms := MakeCMS()

	l := me.NewOrderBookOnULList(7, 3)
	l.InsertOrder("123451", me.SELLSIDE, 10000, 9950, 1000)
	l.InsertOrder("123452", me.SELLSIDE, 10000, 9955, 1000)
	l.InsertOrder("123453", me.SELLSIDE, 10001, 10000, 1000)
	l.InsertOrder("123454", me.SELLSIDE, 10002, 10000, 1000)
	l.InsertOrder("123455", me.SELLSIDE, 10002, 10010, 1000)
	l.InsertOrder("123456", me.SELLSIDE, 10002, 10020, 1000)
	l.InsertOrder("123457", me.SELLSIDE, 10003, 10020, 1000)
	l.InsertOrder("123458", me.SELLSIDE, 10003, 10021, 1000)
	l.InsertOrder("123459", me.SELLSIDE, 10004, 10022, 1000)
	l.InsertOrder("123460", me.SELLSIDE, 10005, 10030, 1000)
	l.InsertOrder("123461", me.SELLSIDE, 10005, 10030, 1000)
	l.InsertOrder("123462", me.SELLSIDE, 10005, 10032, 1000)
	l.InsertOrder("123463", me.SELLSIDE, 10005, 10033, 1000)
	buys, sells := l.GetAllLevels()
	snapshot := OrderBookSnapshot{Buys: buys, Sells: sells, LastTradePrice: 100}
	bytes, _ := cdc.MarshalBinary(snapshot)
	t.Logf("before compress, size is %v", len(bytes))
	kvstore := cms.GetKVStore(common.DexStoreKey)
	key := "testcompress"
	compressAndSave(snapshot, cdc, key, kvstore)
	bz := kvstore.Get([]byte(key))
	assert.NotNil(bz)
	t.Logf("after compress, size is %v", len(bz))
	assert.True(len(bz) < len(bytes))
}

func MakeAddress() (sdk.AccAddress, ed25519.PrivKeyEd25519) {
	privKey := ed25519.GenPrivKey()
	pubKey := privKey.PubKey()
	addr := sdk.AccAddress(pubKey.Address())
	return addr, privKey
}

func TestKeeper_SnapShotOrderBook(t *testing.T) {
	assert := assert.New(t)
	cdc := MakeCodec()
	keeper := MakeKeeper(cdc)
	cms := MakeCMS()
	logger := log.NewTMLogger(os.Stdout)
	ctx := sdk.NewContext(cms, abci.Header{}, true, logger)
	accAdd, _ := MakeAddress()
	msg := NewNewOrderMsg(accAdd, "123456", Side.BUY, "XYZ_BNB", 10200, 300)
	keeper.AddOrder(msg, 42)
	msg = NewNewOrderMsg(accAdd, "123457", Side.BUY, "XYZ_BNB", 10100, 100)
	keeper.AddOrder(msg, 42)
	msg = NewNewOrderMsg(accAdd, "123458", Side.BUY, "XYZ_BNB", 9900, 500)
	keeper.AddOrder(msg, 42)
	msg = NewNewOrderMsg(accAdd, "123459", Side.SELL, "XYZ_BNB", 9800, 100)
	keeper.AddOrder(msg, 42)
	msg = NewNewOrderMsg(accAdd, "123460", Side.SELL, "XYZ_BNB", 9700, 500)
	keeper.AddOrder(msg, 42)
	msg = NewNewOrderMsg(accAdd, "123461", Side.SELL, "XYZ_BNB", 9500, 500)
	keeper.AddOrder(msg, 42)
	msg = NewNewOrderMsg(accAdd, "123462", Side.BUY, "XYZ_BNB", 9600, 150)
	keeper.AddOrder(msg, 42)
	assert.Equal(7, len(keeper.allOrders))
	assert.Equal(1, len(keeper.engines))
	err := keeper.SnapShotOrderBook(43, ctx)
	assert.Nil(err)
	keeper.MarkBreatheBlock(43, time.Now().Unix()*1000, ctx)
	keeper2 := MakeKeeper(cdc)
	pairs := []string{"XYZ_BNB"}
	h, err := keeper2.LoadOrderBookSnapshot(pairs, cms.GetKVStore(common.DexStoreKey), 10)
	assert.Equal(7, len(keeper2.allOrders))
	assert.Equal(int64(9800), keeper2.allOrders["123459"].Price)
	assert.Equal(1, len(keeper2.engines))
	assert.Equal(int64(43), h)
	buys, sells := keeper2.engines["XYZ_BNB"].Book.GetAllLevels()
	assert.Equal(4, len(buys))
	assert.Equal(3, len(sells))
	assert.Equal(int64(10200), buys[0].Price)
	assert.Equal(int64(9600), buys[3].Price)
	assert.Equal(int64(9500), sells[0].Price)
	assert.Equal(int64(9800), sells[2].Price)
}

func TestKeeper_LoadOrderBookSnapshot(t *testing.T) {
	assert := assert.New(t)
	cdc := MakeCodec()
	keeper := MakeKeeper(cdc)
	cms := MakeCMS()
	//logger := log.NewTMLogger(os.Stdout)
	//ctx := sdk.NewContext(cms, abci.Header{}, true, logger)
	kvstore := cms.GetKVStore(common.DexStoreKey)
	pairs := []string{"XYZ_BNB"}
	h, err := keeper.LoadOrderBookSnapshot(pairs, kvstore, 10)
	assert.Zero(h)
	assert.Nil(err)
}

func NewMockBlock(txs []auth.StdTx, height int64, commit *tmtypes.Commit, cdc *wire.Codec) *tmtypes.Block {
	tmTxs := make([]tmtypes.Tx, len(txs))
	for i, tx := range txs {
		tmTxs[i], _ = cdc.MarshalBinary(tx)
	}
	return tmtypes.MakeBlock(height, tmTxs, commit, nil)
}

const BlockPartSize = 65536

func MakeTxFromMsg(msgs []sdk.Msg, accountNumber, seqNum int64, privKey ed25519.PrivKeyEd25519) auth.StdTx {
	fee, _ := sdk.ParseCoin("100 BNB")
	signMsg := auth.StdSignMsg{
		ChainID:       "chainID1",
		AccountNumber: accountNumber,
		Sequence:      seqNum,
		Msgs:          msgs,
		Memo:          "Memo1",
		Fee:           auth.NewStdFee(int64(100), fee), // TODO run simulate to estimate gas?
	}
	sig, _ := privKey.Sign(signMsg.Bytes())
	sigs := []auth.StdSignature{{
		PubKey:        privKey.PubKey(),
		Signature:     sig,
		AccountNumber: accountNumber,
		Sequence:      seqNum,
	}}
	tx := auth.NewStdTx(signMsg.Msgs, signMsg.Fee, sigs, signMsg.Memo)
	return tx
}

func GenerateBlocksAndSave(storedb db.DB, height int64, cdc *wire.Codec) *bc.BlockStore {
	blockStore := bc.NewBlockStore(storedb)
	lastCommit := &tmtypes.Commit{}
	buyerAdd, buyerPrivKey := MakeAddress()
	sellerAdd, sellerPrivKey := MakeAddress()
	txs := make([]auth.StdTx, 7)
	msgs01 := []sdk.Msg{NewNewOrderMsg(buyerAdd, "123456", Side.BUY, "XYZ_BNB", 10200, 300)}
	txs[0] = MakeTxFromMsg(msgs01, int64(100), int64(9001), buyerPrivKey)
	msgs02 := []sdk.Msg{NewNewOrderMsg(buyerAdd, "123457", Side.BUY, "XYZ_BNB", 10100, 100)}
	txs[1] = MakeTxFromMsg(msgs02, int64(100), int64(9002), buyerPrivKey)
	msgs03 := []sdk.Msg{NewNewOrderMsg(sellerAdd, "123459", Side.SELL, "XYZ_BNB", 9800, 100)}
	txs[2] = MakeTxFromMsg(msgs03, int64(1001), int64(7001), sellerPrivKey)
	msgs04 := []sdk.Msg{NewNewOrderMsg(buyerAdd, "123458", Side.BUY, "XYZ_BNB", 9900, 500)}
	txs[3] = MakeTxFromMsg(msgs04, int64(100), int64(9003), buyerPrivKey)
	msgs05 := []sdk.Msg{NewNewOrderMsg(sellerAdd, "123460", Side.SELL, "XYZ_BNB", 9700, 500)}
	txs[4] = MakeTxFromMsg(msgs05, int64(1001), int64(7002), sellerPrivKey)
	msgs06 := []sdk.Msg{NewNewOrderMsg(sellerAdd, "123461", Side.SELL, "XYZ_BNB", 9500, 500)}
	txs[5] = MakeTxFromMsg(msgs06, int64(1001), int64(7003), sellerPrivKey)
	msgs07 := []sdk.Msg{NewNewOrderMsg(buyerAdd, "123462", Side.BUY, "XYZ_BNB", 9600, 150)}
	txs[6] = MakeTxFromMsg(msgs07, int64(100), int64(9004), buyerPrivKey)
	block := NewMockBlock(txs, height, lastCommit, cdc)
	blockParts := block.MakePartSet(BlockPartSize)
	//blockID := tmtypes.BlockID{Hash: block.Hash(), PartsHeader: blockParts.Header()}
	//lastCommit = tmtypes.MakeCommit(block)
	blockStore.SaveBlock(block, blockParts, &tmtypes.Commit{})
	return blockStore
}

func TestKeeper_ReplayOrdersFromBlock(t *testing.T) {
	// assert := assert.New(t)
	// cdc := MakeCodec()
	// keeper := MakeKeeper(cdc)
	// cms := MakeCMS()
	// logger := log.NewTMLogger(os.Stdout)
	// ctx := sdk.NewContext(cms, abci.Header{}, true, logger)
}
