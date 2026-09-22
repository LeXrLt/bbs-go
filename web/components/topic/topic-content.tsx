import {
  HtmlImagePreview,
  PreviewableImage,
} from "@/components/common/image-preview"
import type { Topic } from "@/lib/api/types"
import { cn } from "@/lib/utils"

type TopicContentVariant = "default" | "list"

export function TopicContent({
  topic,
  variant = "default",
}: {
  topic: Topic
  variant?: TopicContentVariant
}) {
  const content = topic.content || topic.summary || ""

  if (!content && !topic.imageList?.length) {
    return null
  }

  const imageCount = topic.imageList?.length || 0
  const previewSrcList = (topic.imageList || []).map(
    (image) => image.url || image.preview || ""
  )
  const isList = variant === "list"

  return (
    <div
      className={cn(
        "pt-0 text-[15px] break-all text-foreground",
        isList ? "contents" : "mx-4 mb-4"
      )}
    >
      {content ? (
        <HtmlImagePreview
          html={content}
          className={cn(
            "bbs-content line-numbers text-base leading-6 wrap-break-word [&_h2]:scroll-mt-20 [&_h3]:scroll-mt-20 [&_h4]:scroll-mt-20 [&_img]:cursor-zoom-in",
            !isList && "antialiased",
            topic.type === 1 && "whitespace-pre-line",
            isList &&
              "contents text-[15px] leading-6 text-foreground/80 sm:text-sm sm:leading-normal [&_li]:my-0 [&_li]:leading-[inherit] [&_p]:mt-0 [&_p]:leading-[inherit] [&_p:last-child]:inline [&_p:not(:last-child)]:mb-3"
          )}
        />
      ) : null}
      {topic.imageList?.length ? (
        <ul className="mt-2.5 ml-0 flex list-none flex-wrap items-center gap-2.5 p-0">
          {topic.imageList.map((image, index) => (
            <li
              key={`${image.preview || image.url || index}`}
              className="cursor-pointer rounded border border-dashed border-border p-0"
            >
              <PreviewableImage
                src={image.preview || image.url || ""}
                previewSrcList={previewSrcList}
                initialIndex={index}
                alt=""
                className={`m-0 block cursor-zoom-in overflow-hidden rounded object-cover p-0 transition-all duration-500 ease-out hover:scale-[1.04] ${
                  imageCount <= 1
                    ? "h-[160px] w-[160px] sm:h-[210px] sm:w-[210px]"
                    : imageCount === 2
                      ? "h-[128px] w-[128px] sm:h-[180px] sm:w-[180px]"
                      : "h-[94px] w-[94px] sm:h-[120px] sm:w-[120px]"
                }`}
              />
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  )
}
