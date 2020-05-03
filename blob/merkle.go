package blob

import (
	"bytes"
	"ddrp/crypto"
	"github.com/pkg/errors"
	"io"
	"math"
	"math/bits"
	"strings"
)

const (
	MerkleTreeHeight = 8
	MerkleProofLen   = 8 * 32

	SubsectorSize        = 4096
	SubsectorCountBlob   = Size / SubsectorSize
	SubsectorCountSector = SectorLen / SubsectorSize
	SubsectorProofLevel  = 8
)

var (
	precomputes = map[crypto.Hash]crypto.Hash{
		// merkle tree levels, starting from base
		// assuming 4096 byte sub-sectors
		{0x68, 0x6e, 0xde, 0x92, 0x88, 0xc3, 0x91, 0xe7, 0xe0, 0x50, 0x26, 0xe5, 0x6f, 0x2f, 0x91, 0xbf, 0xd8, 0x79, 0x98, 0x7a, 0x04, 0x0e, 0xa9, 0x84, 0x45, 0xda, 0xbc, 0x76, 0xf5, 0x5b, 0x8e, 0x5f}: {0x49, 0xe4, 0xb8, 0x0d, 0x5b, 0x7d, 0x8d, 0x93, 0x22, 0x48, 0x25, 0xf2, 0x6c, 0x45, 0x98, 0x7e, 0x10, 0x7b, 0xbf, 0x2f, 0x87, 0x1d, 0x4e, 0x56, 0x36, 0xac, 0x55, 0x0f, 0xf1, 0x25, 0xe0, 0x82},
		{0x49, 0xe4, 0xb8, 0x0d, 0x5b, 0x7d, 0x8d, 0x93, 0x22, 0x48, 0x25, 0xf2, 0x6c, 0x45, 0x98, 0x7e, 0x10, 0x7b, 0xbf, 0x2f, 0x87, 0x1d, 0x4e, 0x56, 0x36, 0xac, 0x55, 0x0f, 0xf1, 0x25, 0xe0, 0x82}: {0xb7, 0x95, 0x51, 0x37, 0x10, 0x7e, 0x8c, 0x87, 0x98, 0x94, 0xa9, 0x47, 0xa2, 0x35, 0xec, 0xd2, 0xa4, 0x22, 0x5a, 0x6c, 0x82, 0x12, 0x97, 0x6e, 0x97, 0x6c, 0x50, 0x31, 0x9b, 0x59, 0x31, 0xb3},
		{0xb7, 0x95, 0x51, 0x37, 0x10, 0x7e, 0x8c, 0x87, 0x98, 0x94, 0xa9, 0x47, 0xa2, 0x35, 0xec, 0xd2, 0xa4, 0x22, 0x5a, 0x6c, 0x82, 0x12, 0x97, 0x6e, 0x97, 0x6c, 0x50, 0x31, 0x9b, 0x59, 0x31, 0xb3}: {0x47, 0x19, 0x5c, 0x0c, 0xa9, 0x4e, 0x02, 0x20, 0xcd, 0x01, 0xbe, 0x88, 0x32, 0x00, 0xfd, 0xbf, 0x71, 0x48, 0x13, 0x64, 0x94, 0x2b, 0xa1, 0xe8, 0xd3, 0xef, 0x4c, 0x9a, 0x3d, 0xc4, 0xb6, 0xa5},
		{0x47, 0x19, 0x5c, 0x0c, 0xa9, 0x4e, 0x02, 0x20, 0xcd, 0x01, 0xbe, 0x88, 0x32, 0x00, 0xfd, 0xbf, 0x71, 0x48, 0x13, 0x64, 0x94, 0x2b, 0xa1, 0xe8, 0xd3, 0xef, 0x4c, 0x9a, 0x3d, 0xc4, 0xb6, 0xa5}: {0x53, 0x2a, 0x12, 0xf0, 0x9f, 0xeb, 0xf8, 0x52, 0x14, 0x19, 0x95, 0x99, 0x73, 0xad, 0x53, 0x46, 0x94, 0x4c, 0x2b, 0x22, 0xbf, 0x76, 0x4d, 0x0e, 0x1a, 0x34, 0x25, 0x5b, 0x65, 0x64, 0xfe, 0x4b},
		{0x53, 0x2a, 0x12, 0xf0, 0x9f, 0xeb, 0xf8, 0x52, 0x14, 0x19, 0x95, 0x99, 0x73, 0xad, 0x53, 0x46, 0x94, 0x4c, 0x2b, 0x22, 0xbf, 0x76, 0x4d, 0x0e, 0x1a, 0x34, 0x25, 0x5b, 0x65, 0x64, 0xfe, 0x4b}: {0xff, 0xe2, 0xcf, 0x7e, 0xcd, 0x1b, 0x99, 0x32, 0x35, 0x74, 0x6b, 0xe2, 0x1e, 0x91, 0xc8, 0xe6, 0x1a, 0x1e, 0x22, 0xda, 0xce, 0x98, 0x50, 0x91, 0x25, 0x85, 0x41, 0x65, 0x01, 0xe9, 0x84, 0x47},
		{0xff, 0xe2, 0xcf, 0x7e, 0xcd, 0x1b, 0x99, 0x32, 0x35, 0x74, 0x6b, 0xe2, 0x1e, 0x91, 0xc8, 0xe6, 0x1a, 0x1e, 0x22, 0xda, 0xce, 0x98, 0x50, 0x91, 0x25, 0x85, 0x41, 0x65, 0x01, 0xe9, 0x84, 0x47}: {0x60, 0x52, 0x2e, 0x01, 0x34, 0x1f, 0xe9, 0x62, 0x54, 0x66, 0x8b, 0xa1, 0xbc, 0x2c, 0x79, 0xdd, 0x6f, 0xc6, 0x6b, 0x37, 0x84, 0xc2, 0xeb, 0x39, 0xd4, 0xf0, 0x73, 0x19, 0xc6, 0x23, 0x26, 0x57},
		{0x60, 0x52, 0x2e, 0x01, 0x34, 0x1f, 0xe9, 0x62, 0x54, 0x66, 0x8b, 0xa1, 0xbc, 0x2c, 0x79, 0xdd, 0x6f, 0xc6, 0x6b, 0x37, 0x84, 0xc2, 0xeb, 0x39, 0xd4, 0xf0, 0x73, 0x19, 0xc6, 0x23, 0x26, 0x57}: {0xae, 0x01, 0x82, 0xab, 0x78, 0x03, 0xfc, 0x44, 0xd0, 0x85, 0xc1, 0xc7, 0x34, 0xa5, 0x52, 0xff, 0xfd, 0xb0, 0xf7, 0x44, 0x17, 0x9f, 0x0d, 0x95, 0xbd, 0x60, 0xdd, 0x6f, 0x8f, 0x18, 0x18, 0xaf},
		{0xae, 0x01, 0x82, 0xab, 0x78, 0x03, 0xfc, 0x44, 0xd0, 0x85, 0xc1, 0xc7, 0x34, 0xa5, 0x52, 0xff, 0xfd, 0xb0, 0xf7, 0x44, 0x17, 0x9f, 0x0d, 0x95, 0xbd, 0x60, 0xdd, 0x6f, 0x8f, 0x18, 0x18, 0xaf}: {0xf3, 0x4c, 0x7d, 0x70, 0xb6, 0x52, 0xeb, 0xa4, 0x8e, 0x02, 0xb4, 0x71, 0x7e, 0x1f, 0x0a, 0x2b, 0xe9, 0x33, 0x7b, 0x07, 0x51, 0xcc, 0xf7, 0xbf, 0x36, 0x44, 0x67, 0x4c, 0xbe, 0x64, 0x23, 0xd5},
		{0xf3, 0x4c, 0x7d, 0x70, 0xb6, 0x52, 0xeb, 0xa4, 0x8e, 0x02, 0xb4, 0x71, 0x7e, 0x1f, 0x0a, 0x2b, 0xe9, 0x33, 0x7b, 0x07, 0x51, 0xcc, 0xf7, 0xbf, 0x36, 0x44, 0x67, 0x4c, 0xbe, 0x64, 0x23, 0xd5}: {0x32, 0x00, 0xb9, 0x9b, 0xfc, 0xd8, 0x2c, 0x64, 0xc8, 0x18, 0xb1, 0xa2, 0xb2, 0x6e, 0x14, 0xbf, 0x78, 0x4f, 0xe9, 0x18, 0x8a, 0x55, 0x9b, 0x6b, 0x38, 0xa6, 0xdd, 0xa4, 0xfb, 0x55, 0x31, 0x47},
		{0x32, 0x00, 0xb9, 0x9b, 0xfc, 0xd8, 0x2c, 0x64, 0xc8, 0x18, 0xb1, 0xa2, 0xb2, 0x6e, 0x14, 0xbf, 0x78, 0x4f, 0xe9, 0x18, 0x8a, 0x55, 0x9b, 0x6b, 0x38, 0xa6, 0xdd, 0xa4, 0xfb, 0x55, 0x31, 0x47}: {0xb8, 0x6b, 0xe3, 0xa8, 0xb2, 0x88, 0xb0, 0xef, 0x0b, 0x7e, 0xe5, 0xa9, 0x85, 0x2d, 0x11, 0x81, 0x67, 0xa5, 0x0c, 0x84, 0x71, 0xbf, 0xb9, 0xfb, 0x8f, 0x0c, 0x79, 0x92, 0xf4, 0x52, 0xc7, 0x9c},
		{0xb8, 0x6b, 0xe3, 0xa8, 0xb2, 0x88, 0xb0, 0xef, 0x0b, 0x7e, 0xe5, 0xa9, 0x85, 0x2d, 0x11, 0x81, 0x67, 0xa5, 0x0c, 0x84, 0x71, 0xbf, 0xb9, 0xfb, 0x8f, 0x0c, 0x79, 0x92, 0xf4, 0x52, 0xc7, 0x9c}: {0xaa, 0xbd, 0xcb, 0xb2, 0x3b, 0xfc, 0xea, 0xfe, 0x71, 0xf3, 0x83, 0x4a, 0x17, 0xec, 0x1d, 0x24, 0xbd, 0x4e, 0xf2, 0xda, 0xed, 0x68, 0xd2, 0xcc, 0xc3, 0xc2, 0x98, 0x9f, 0xd0, 0x92, 0xe3, 0x53},
		{0xaa, 0xbd, 0xcb, 0xb2, 0x3b, 0xfc, 0xea, 0xfe, 0x71, 0xf3, 0x83, 0x4a, 0x17, 0xec, 0x1d, 0x24, 0xbd, 0x4e, 0xf2, 0xda, 0xed, 0x68, 0xd2, 0xcc, 0xc3, 0xc2, 0x98, 0x9f, 0xd0, 0x92, 0xe3, 0x53}: {0x7d, 0x1e, 0x84, 0xe6, 0x2d, 0x7e, 0xc9, 0xf6, 0xbc, 0x3f, 0x88, 0x66, 0x75, 0x73, 0x3d, 0xa0, 0x6d, 0xaf, 0x02, 0x77, 0xad, 0x8f, 0xfc, 0xf7, 0xc5, 0x39, 0x93, 0x8c, 0xa5, 0xee, 0x9c, 0xf2},
	}

	zero4kSector     = make([]byte, 4096)
	zero4kSectorHash = crypto.Hash{0x68, 0x6e, 0xde, 0x92, 0x88, 0xc3, 0x91, 0xe7, 0xe0, 0x50, 0x26, 0xe5, 0x6f, 0x2f, 0x91, 0xbf, 0xd8, 0x79, 0x98, 0x7a, 0x04, 0x0e, 0xa9, 0x84, 0x45, 0xda, 0xbc, 0x76, 0xf5, 0x5b, 0x8e, 0x5f}

	EmptyBlobMerkleRoot = crypto.Hash{0x7d, 0x1e, 0x84, 0xe6, 0x2d, 0x7e, 0xc9, 0xf6, 0xbc, 0x3f, 0x88, 0x66, 0x75, 0x73, 0x3d, 0xa0, 0x6d, 0xaf, 0x02, 0x77, 0xad, 0x8f, 0xfc, 0xf7, 0xc5, 0x39, 0x93, 0x8c, 0xa5, 0xee, 0x9c, 0xf2}
	EmptyBlobBaseHash   = crypto.Hash{0x53, 0x2a, 0x12, 0xf0, 0x9f, 0xeb, 0xf8, 0x52, 0x14, 0x19, 0x95, 0x99, 0x73, 0xad, 0x53, 0x46, 0x94, 0x4c, 0x2b, 0x22, 0xbf, 0x76, 0x4d, 0x0e, 0x1a, 0x34, 0x25, 0x5b, 0x65, 0x64, 0xfe, 0x4b}
	ZeroMerkleBase      MerkleBase
)

type MerkleBase [256]crypto.Hash

func (m MerkleBase) Encode(w io.Writer) error {
	for _, h := range m {
		if _, err := w.Write(h[:]); err != nil {
			return err
		}
	}
	return nil
}

func (m *MerkleBase) Decode(r io.Reader) error {
	var res MerkleBase
	var hash crypto.Hash
	for i := 0; i < len(res); i++ {
		if _, err := r.Read(hash[:]); err != nil {
			return err
		}
		res[i] = hash
	}

	*m = res
	return nil
}

func (m MerkleBase) DiffWith(other MerkleBase) []uint8 {
	if m == other {
		return nil
	}

	var out []uint8
	for i := 0; i < len(m); i++ {
		if m[i] != other[i] {
			out = append(out, uint8(i))
		}
	}
	return out
}

type MerkleProof [MerkleProofLen]byte

func (m MerkleProof) Encode(w io.Writer) error {
	_, err := w.Write(m[:])
	return err
}

func (m *MerkleProof) Decode(r io.Reader) error {
	var buf MerkleProof
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return err
	}
	*m = buf
	return nil
}

func MakeSectorProof(tree MerkleTree, sectorID uint8) MerkleProof {
	var proof MerkleProof
	var buf bytes.Buffer
	pos := sectorID
	for i := SubsectorProofLevel; i >= 1; i-- {
		level := tree[i]
		if pos%2 == 0 {
			buf.Write(level[pos+1].Bytes())
		} else {
			buf.Write(level[pos-1].Bytes())
		}
		pos = pos / 2
	}
	copy(proof[:], buf.Bytes())
	return proof
}

func VerifySectorProof(sector Sector, sectorID uint8, merkleRoot crypto.Hash, proof MerkleProof) bool {
	currHash := HashSector(sector)
	pos := sectorID
	pRdr := bytes.NewReader(proof[:])
	var proofHash crypto.Hash
	for i := 0; i < MerkleTreeHeight; i++ {
		_, err := io.ReadFull(pRdr, proofHash[:])
		if err != nil {
			return false
		}
		if pos%2 == 0 {
			currHash = hashLevel(currHash, proofHash)
		} else {
			currHash = hashLevel(proofHash, currHash)
		}
		pos = pos / 2
	}

	return currHash == merkleRoot
}

func HashSector(sector Sector) crypto.Hash {
	sectorTree, err := NewMerkleTreeFromReader(bytes.NewReader(sector[:]), SubsectorCountSector, SubsectorSize)
	if err != nil {
		// should never happen
		panic(err)
	}
	return sectorTree.Root()
}

type MerkleTree [][]crypto.Hash

func Merkleize(br io.Reader) (MerkleTree, error) {
	return NewMerkleTreeFromReader(br, SubsectorCountBlob, SubsectorSize)
}

func MakeTreeFromBase(base MerkleBase) MerkleTree {
	tree, err := newMerkleTreeFromHashedLeaves(base[:])
	if err != nil {
		/// should never happen
		panic(err)
	}
	return tree
}

func NewMerkleTreeFromReader(r io.Reader, leafCount int, leafSize int) (MerkleTree, error) {
	if leafCount > math.MaxUint32 {
		return nil, errors.New("leafCount must be less than math.MaxUint32")
	}
	if bits.OnesCount64(uint64(leafCount)) != 1 {
		return nil, errors.New("leafCount must be a power of two")
	}

	buf := make([]byte, leafSize)
	var base []crypto.Hash
	for i := 0; i < leafCount; i++ {
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		base = append(base, hashLeaf(buf))
	}
	return newMerkleTreeFromHashedLeaves(base)
}

func newMerkleTreeFromHashedLeaves(base []crypto.Hash) (MerkleTree, error) {
	if bits.OnesCount64(uint64(len(base))) != 1 {
		return nil, errors.New("base must have a power of two length")
	}

	tree := [][]crypto.Hash{
		base,
	}
	for len(tree[0]) > 1 {
		var level []crypto.Hash
		for i := 0; i < len(tree[0]); i += 2 {
			left := tree[0][i]
			right := tree[0][i+1]
			level = append(level, hashLevel(left, right))
		}
		tree = append([][]crypto.Hash{level}, tree...)
	}
	return tree, nil
}

func (t MerkleTree) Root() crypto.Hash {
	return t[0][0]
}

func (t MerkleTree) ProtocolBase() MerkleBase {
	var out MerkleBase
	data := t.Level(8)
	if len(data) != len(out) {
		panic("invalid tree level")
	}
	for i := 0; i < len(out); i++ {
		out[i] = data[i]
	}
	return out
}

func (t MerkleTree) Height() int {
	if len(t) == 0 {
		panic("trying to get height of nil merkle tree")
	}

	return len(t) - 1
}

func (t MerkleTree) Level(i int) []crypto.Hash {
	level := t[i]
	out := make([]crypto.Hash, len(level))
	for i := 0; i < len(level); i++ {
		out[i] = level[i]
	}
	return out
}

func (t MerkleTree) String() string {
	var buf strings.Builder
	for i := len(t) - 1; i >= 0; i-- {
		level := t[i]
		for _, hash := range level {
			if i < len(t)-1 {
				buf.WriteString(strings.Repeat("  ", len(t)-i))
				buf.WriteString("﹂")
			}

			buf.WriteString(hash.String())
			buf.WriteRune('\n')
		}
	}
	return buf.String()
}

func (t MerkleTree) Encode(w io.Writer) error {
	if _, err := w.Write([]byte{uint8(t.Height())}); err != nil {
		return err
	}
	for i := len(t) - 1; i >= 0; i-- {
		level := t[i]
		for _, node := range level {
			if _, err := w.Write(node.Bytes()); err != nil {
				return err
			}
		}
	}
	return nil
}

func (t *MerkleTree) Decode(r io.Reader) error {
	heightB := make([]byte, 1, 1)
	if _, err := io.ReadFull(r, heightB); err != nil {
		return err
	}
	height := int(heightB[0])
	if height > 32 {
		return errors.New("refusing decode tree over 32 levels deep")
	}
	toRead := 1 << uint8(height)
	var tree MerkleTree
	for {
		var level []crypto.Hash
		for i := 0; i < toRead; i++ {
			var h crypto.Hash
			if err := h.Decode(r); err != nil {
				return err
			}
			level = append(level, h)
		}
		tree = append([][]crypto.Hash{level}, tree...)
		if toRead == 1 {
			break
		}
		toRead = toRead / 2
	}
	*t = tree
	return nil
}

func hashLeaf(in []byte) crypto.Hash {
	if bytes.Equal(zero4kSector, in) {
		return zero4kSectorHash
	}

	return crypto.Blake2B256(in)
}

func hashLevel(left crypto.Hash, right crypto.Hash) crypto.Hash {
	precompRes, hasPrecomp := precomputes[left]
	if hasPrecomp && left == right {
		return precompRes
	}

	return crypto.Blake2B256(left[:], right[:])
}