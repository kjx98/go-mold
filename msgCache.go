package MoldUDP

const (
	maxPageMsg    = 0x100000
	pageShift     = 20
	pageIncrement = 16
)

type msgPage [maxPageMsg]*Message

type msgCache struct {
	nPage     int
	maxPageNo int
	msgPages  []msgPage
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
		for page >= mc.nPage {
			msgPP := make([]msgPage, pageIncrement)
			mc.msgPages = append(mc.msgPages, msgPP...)
			mc.nPage += pageIncrement
		}
	}
	if page > mc.maxPageNo {
		mc.maxPageNo = page
	}
	ret := mc.msgPages[page][off] != nil
	mc.msgPages[page][off] = msg
	return ret
}

func (mc *msgCache) IsNil(seqNo uint64) bool {
	page := int(seqNo >> pageShift)
	off := int(seqNo & (maxPageMsg - 1))
	if page >= mc.nPage {
		return true
	}
	if mc.msgPages[page][off] == nil {
		return true
	}
	return false
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
	getCount := func(page, off int) (res int) {
		for mc.msgPages[page][off] != nil {
			res++
			off++
			if off >= maxPageMsg {
				off = 0
				page++
				if page >= mc.nPage {
					break
				}
			}
		}
		return
	}
	cnt := getCount(page, off)
	if cnt == 0 {
		return []Message{}
	}
	ret := make([]Message, cnt)
	i := 0
	for mc.msgPages[page][off] != nil {
		ret[i] = *mc.msgPages[page][off]
		i++
		if i >= cnt {
			break
		}
		off++
		if off >= maxPageMsg {
			off = 0
			page++
		}
	}
	return ret
}
