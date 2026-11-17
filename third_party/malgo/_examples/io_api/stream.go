package io_api

import (
	"context"

	"github.com/gen2brain/malgo"
)

func stream(ctx context.Context, abortChan chan error, deviceConfig malgo.DeviceConfig, deviceCallbacks malgo.DeviceCallbacks) error {
	mctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = mctx.Uninit()
		mctx.Free()
	}()

	device, err := malgo.InitDevice(mctx.Context, deviceConfig, deviceCallbacks)
	if err != nil {
		return err
	}
	defer device.Uninit()

	err = device.Start()
	if err != nil {
		return err
	}

	ctxChan := ctx.Done()
	if ctxChan != nil {
		select {
		case <-ctxChan:
			err = ctx.Err()
		case err = <-abortChan:
		}
	} else {
		err = <-abortChan
	}

	return err
}
