package rag

import (
	"context"
	"github.com/walkthrough-assistance/data-preparation/model"
	writer "github.com/walkthrough-assistance/data-preparation/rag"
)

func (r *Repository) Save(ctx context.Context, d model.Document) (model.Result, error) {
	w, err := writer.NewRepository(r.root)
	if err != nil {
		return model.Result{}, err
	}
	return w.Save(ctx, d)
}
