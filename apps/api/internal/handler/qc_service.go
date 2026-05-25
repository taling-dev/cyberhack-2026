package handler

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/db"
	qcv1 "github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/qc/v1"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/qc/v1/qcv1connect"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/storage"
)

var _ qcv1connect.QCServiceHandler = (*QCService)(nil)

type QCService struct {
	q     *db.Queries
	minio *storage.MinIOClient
}

func NewQCService(q *db.Queries, minio *storage.MinIOClient) *QCService {
	return &QCService{q: q, minio: minio}
}

func (s *QCService) CreateQCUploadUrl(ctx context.Context, req *connect.Request[qcv1.CreateQCUploadUrlRequest]) (*connect.Response[qcv1.CreateQCUploadUrlResponse], error) {
	msg := req.Msg

	if msg.LotId == "" || msg.Filename == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("lot_id and filename are required"))
	}

	contentType := msg.ContentType
	if contentType == "" {
		contentType = "image/jpeg"
	}

	ext := ".jpg"
	if contentType == "image/png" {
		ext = ".png"
	}

	objectKey := fmt.Sprintf("%s/%s%s", msg.LotId, uuid.NewString(), ext)
	expiry := 15 * time.Minute

	uploadURL, err := s.minio.PresignedPutURL(ctx, storage.QCImagesBucket, objectKey, contentType, expiry)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to generate upload URL: %w", err))
	}

	expiresAt := time.Now().Add(expiry).Unix()

	return connect.NewResponse(&qcv1.CreateQCUploadUrlResponse{
		ObjectKey:    objectKey,
		UploadUrl:    uploadURL,
		ExpiresAtUnix: expiresAt,
	}), nil
}

func (s *QCService) CreateQCJob(ctx context.Context, req *connect.Request[qcv1.CreateQCJobRequest]) (*connect.Response[qcv1.CreateQCJobResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("implemented in Task 9"))
}

func (s *QCService) GetQCJob(ctx context.Context, req *connect.Request[qcv1.GetQCJobRequest]) (*connect.Response[qcv1.GetQCJobResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("implemented in Task 10"))
}

func (s *QCService) GetQCResult(ctx context.Context, req *connect.Request[qcv1.GetQCResultRequest]) (*connect.Response[qcv1.GetQCResultResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("implemented in Task 10"))
}

func (s *QCService) ReviewQC(ctx context.Context, req *connect.Request[qcv1.ReviewQCRequest]) (*connect.Response[qcv1.ReviewQCResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("implemented in Task 11"))
}

func (s *QCService) RetryQCJob(ctx context.Context, req *connect.Request[qcv1.RetryQCJobRequest]) (*connect.Response[qcv1.RetryQCJobResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("implemented in Task 19"))
}
