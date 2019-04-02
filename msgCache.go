package MoldUDP

const (
	maxPageMsg    = 0x100000
	pageShift     = 20
	pageIncrement = 16
)

type msgPage [maxPageMsg]*Message

type msgCache struct {
	nPage    int
	msgPages []msgPage
}

func (mc *msgCache) Init() {
	mc.nPage = pageIncrement
	mc.msgPages = make([]msgPage, pageIncrement)
}

// Upset  update or insert
//	return true for update
func (mc *msgCache) Upset(seqNo uint64, msg *Message) bool {
	page := int(seqNo >> pageShift)
	off := int(seqNo & (maxPageMsg - 1))
	if page >= mc.nPage {
		for page > mc.nPage {
			msgPP := make([]msgPage, pageIncrement)
			mc.msgPages = append(mc.msgPages, msgPP...)
			mc.nPage += pageIncrement
		}
	}
	ret := mc.msgPages[page][off] != nil
	mc.msgPages[page][off] = msg
	return ret
}

func (mc *msgCache) Merge(seqNo uint64) []Message {
	page := int(seqNo >> pageShift)
	off := int(seqNo & (maxPageMsg - 1))
	if page >= mc.nPage {
		return nil
	}
	if mc.msgPages[page][off] == nil {
		return nil
	}
	ret := []Message{}
	nPP := 0
	for mc.msgPages[page][off] != nil {
		ret = append(ret, *mc.msgPages[page][off])
		off++
		if off >= maxPageMsg {
			off = 0
			nPP++
			page++
			if nPP > 2 {
				break
			}
		}
	}
	return ret
}
