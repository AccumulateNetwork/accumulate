package protocol

func (s *HashSet) Add(hash [32]byte) {
	// TODO Use a binary search

	for _, h := range s.Hashes {
		if h == hash {
			return
		}
	}

	s.Hashes = append(s.Hashes, hash)
}

func (s *HashSet) Remove(hash [32]byte) {
	// TODO Use a binary search

	for i, h := range s.Hashes {
		if h != hash {
			continue
		}

		s.Hashes = append(s.Hashes[:i], s.Hashes[i+1:]...)
		break
	}
}
