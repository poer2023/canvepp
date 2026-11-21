package main

func (a *desktop) cancelRun(id string) {
	go func() {
		err := a.store.CancelRun(id)
		r, readErr := a.store.NativeRun(id)
		a.publish(func() {
			if err != nil {
				a.message(err.Error())
				return
			}
			for i := range a.runs {
				if a.runs[i].ID == id {
					a.runs[i].Cancelled = true
					if a.runs[i].Status == "queued" {
						a.runs[i].Status = "cancelled"
					}
				}
			}
			if readErr == nil {
				if e := a.editors[r.ProjectID]; e != nil {
					op := cancelOperation(r)
					seen := false
					for _, existing := range e.Events {
						seen = seen || existing.ID == op.ID
					}
					if !seen {
						e.Events = append(e.Events, op)
					}
				}
			}
			a.runRevision++
			a.message("已请求取消，已返回的图片仍会保留")
		})
	}()
}
