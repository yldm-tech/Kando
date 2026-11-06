package main

import (
	"errors"
	"strings"

	"lingocut/packages/core"
)

// errBusy is Enqueue's answer while a job runs. Jobs run one at a time and, until the queue keeps the others waiting, a source given while one runs is not added.
var errBusy = errors.New("已有视频在处理，等它完成后再添加")

// Enqueue adds a job for each source, a link or a local video's file:// URL, to the open library in the language the settings name, and runs it. One job runs at a time: Enqueue runs the first source and refuses the others while it runs, and refuses all of them while another job runs.
func (a *app) Enqueue(sources ...string) error {
	if len(sources) == 0 {
		return core.ErrEmptySource
	}
	for _, source := range sources {
		if a.running() {
			return errBusy
		}
		job, err := a.newJob(source)
		if err != nil {
			return err
		}
		a.run(job)
	}
	return nil
}

// Cancel stops job id if it is the one running. Its card stays, marked 已取消.
func (a *app) Cancel(id string) {
	job, ok := a.library.Find(id)
	if !ok || job.Stage.Done() || id != a.selectedID {
		return
	}
	a.stop()
}

// newJob makes the job for source in the open library and the target language, or returns why it cannot.
func (a *app) newJob(source string) (core.Job, error) {
	if strings.TrimSpace(a.snap().LibraryDir) == "" {
		return core.Job{}, errors.New("先创建或导入一个资料库")
	}
	job, err := core.NewJob(source)
	if err != nil {
		return core.Job{}, err
	}
	language := strings.TrimSpace(a.language)
	if language == "" {
		return core.Job{}, errors.New("请填写目标语言")
	}
	job.Language = language
	return job, nil
}
