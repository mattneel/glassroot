package githubreceiver

func validateReceiverID(id string) error {
	if id == "" || len(id) > 64 || id[0] < 'a' || id[0] > 'z' || hasControl(id) {
		return errCode(CodeInvalidReceiverID, "receiver", "receiver id rejected", nil)
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-') {
			return errCode(CodeInvalidReceiverID, "receiver", "receiver id rejected", nil)
		}
	}
	return nil
}
