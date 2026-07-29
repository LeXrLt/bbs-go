export const COMMENT_CONTENT_TYPE = "markdown" as const

type CommentImage = {
  url?: string
  preview?: string
}

type CommentCreateFieldsInput = {
  entityType: string
  entityId: string | number
  quoteId?: number
  content: string
  imageList: CommentImage[]
}

export function buildCommentCreateFields({
  entityType,
  entityId,
  quoteId,
  content,
  imageList,
}: CommentCreateFieldsInput) {
  return {
    entityType,
    entityId,
    quoteId,
    contentType: COMMENT_CONTENT_TYPE,
    content,
    imageList: imageList.length ? JSON.stringify(imageList) : "",
  }
}
