package validator

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/asm"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/core/vm/runtime"
	"math/big"
	"testing"
)

type EvmOpcodes struct {
	opstack [][32]byte
	exestack []byte
	ins map[vm.OpCode]interface{}
}

func NewEvmStack() *EvmOpcodes {
	ret := EvmOpcodes{}
	ret.ins = make(map[vm.OpCode]interface{})
	//ret.opstack = make([][]byte,1)

	//ret.stack = make([]byte,1)
	//push 2 -- 32 can be derrived if vm.IsPush(opcode) == true
	//if is a push then amount to push on stack is (opcode - vm.Push1)
	ret.ins[vm.PUSH1] = ret.PUSH
	ret.ins[vm.MSTORE] = ret.MSTORE

	//ret.ins[vm.]
	return &ret
}
func (evm *EvmOpcodes) IsOpstackEmpty() bool {
	return len(evm.opstack) == 0
}

func (evm *EvmOpcodes) PUSH(b []byte) error {
	evm.opstack = append(evm.opstack, [32]byte{})
	copy(evm.opstack[len(evm.opstack)-1][:], b)
	return nil
}

func (evm *EvmOpcodes) pop() ([]byte, bool) {
	if evm.IsOpstackEmpty() {
		return nil, false
	} else {
		index := len(evm.opstack) - 1 // Get the index of the top most element.
		element := evm.opstack[index] // Index into the slice and obtain the element.
		evm.opstack = evm.opstack[:index] // Remove it from the stack by slicing it off.
		return element[:], true
	}
}

//store 256 bytes of data at offset
func (evm *EvmOpcodes) MSTORE() error{
	param2, found := evm.pop()
	if !found {
		return fmt.Errorf("invalid opcode in MSTORE")
	}
	value := binary.LittleEndian.Uint64(param2)

	param1, found := evm.pop()
	if !found {
		return fmt.Errorf("invalid opcode in MSTORE")
	}

	offset := binary.LittleEndian.Uint64(param1)
	if 0x100 + offset > 0xFFFF {
		return fmt.Errorf("cannot reserve memory, out of bounds MSTORE offset %X at value  %X",offset,value)
	}

	s := cap(evm.exestack)

	if s < int(offset + 0x100) {
		b := make([]byte, offset + 0x100)
		copy(b, param2)
		evm.exestack = append(evm.exestack, b...)
	}

	return nil
}


func (evm *EvmOpcodes) Add(a *big.Int, b *big.Int) *big.Int {
	r := big.Int{}
	r.Add(a,b)
	return &r
}

func (evm *EvmOpcodes) Sub(a *big.Int, b *big.Int) *big.Int {
	r := big.Int{}
	r.Sub(a,b)
	return &r
}

func (evm *EvmOpcodes) Mul(a *big.Int, b *big.Int) *big.Int {
	r := big.Int{}
	r.Mul(a,b)
	return &r
}


func TestEVM(t *testing.T) {
	//compiler := asm.NewCompiler(true)
	//compiler.Feed()
	//compiler.Compile()

	evm := NewEvmStack()

	cnt := 0
	script, _ := hex.DecodeString("60806040526001600055348015601457600080fd5b5060358060226000396000f3fe")

	//it := asm.NewInstructionIterator(script)
	for it := asm.NewInstructionIterator(script); it.Next() ; {
        if it.Op().IsPush() {
        	fmt.Printf("Pushing %X\n", it.Arg())
			evm.PUSH(it.Arg())
		} else if it.Op() == vm.MSTORE {
			evm.MSTORE()
		}
		cnt++
	}
	ret, _, err := runtime.Execute(common.Hex2Bytes("6060604052600a8060106000396000f360606040526008565b00"), nil, nil)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(ret)


	//if err := it.Error(); err != nil {
	//	t.Errorf("Expected 2, but encountered error %v instead.", err)
	//}
	if cnt != 2 {
		t.Errorf("Expected 2, but got %v instead.", cnt)
	}

}

