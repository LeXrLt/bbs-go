import { useLoaderData, useParams } from "react-router"

import { useCurrentUser } from "@/components/app/app-provider"
import { RequireUser } from "@/components/auth/require-user"
import { AttachmentPreviewPage } from "@/components/topic/attachment-preview-page"
import { noindexRouteMeta } from "@/lib/seo"

import { requireUser, requireUserClient } from "../route-helpers/auth"
import { loadTopicDetail } from "../route-helpers/loaders"

type RouteArgs = {
  request: Request
  params: { id?: string; attachmentId?: string }
}

export async function loader(args: RouteArgs) {
  await requireUser(args)
  return loadTopicDetail({ request: args.request, id: args.params.id || "" })
}

export async function clientLoader(args: RouteArgs) {
  await requireUserClient(args)
  return loadTopicDetail({ id: args.params.id || "" })
}

export function meta({
  matches,
}: {
  matches: Array<{ data?: unknown; loaderData?: unknown }>
}) {
  return noindexRouteMeta(matches, "Attachment preview", "附件预览")
}

export default function AttachmentPreviewRoute() {
  const topic = useLoaderData<typeof loader>()
  const currentUser = useCurrentUser()
  const { attachmentId = "" } = useParams()
  const redirectPath = `/topic/${encodeURIComponent(topic.id)}/attachment/${encodeURIComponent(attachmentId)}/preview`

  return (
    <RequireUser initialUser={currentUser} redirectPath={redirectPath}>
      <AttachmentPreviewPage topic={topic} attachmentId={attachmentId} />
    </RequireUser>
  )
}
