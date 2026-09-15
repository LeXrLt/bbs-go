import { redirect } from "react-router"

import { apiFetch } from "@/lib/api/client"
import type {
  Article,
  PageData,
  Tag,
  Topic,
  Category,
  UserSummary,
} from "@/lib/api/types"
import {
  DAILY_REPORT_CATEGORY_NAME,
  findDailyReportCategory,
  parseDailyReportFilters,
} from "@/lib/daily-reports"
import {
  normalizeTopicRoleName,
  TOPIC_ROLE_NAME_PARAM,
  type TopicRoleName,
} from "@/lib/topic-role-filter"

import { getCurrentUser } from "./auth"

export type TopicListRouteData = {
  topics: PageData<Topic>
  categories: Category[]
  category?: Category | null
  tag?: Tag | null
  roleName?: TopicRoleName | ""
  roleAfter?: string
}

export type DailyReportRouteData = TopicListRouteData & {
  authors: UserSummary[]
  filters: ReturnType<typeof parseDailyReportFilters>
}

const qaStatusOptions = ["", "unsolved", "solved"]
const sortOptions = ["latestEdit", "latestReply"]

export function resolveCategoryId(id?: string) {
  if (id === "newest") return 0
  if (id === "recommend") return -1
  if (id === "feed") return -2
  const parsed = Number.parseInt(id || "", 10)
  return Number.isNaN(parsed) ? 0 : parsed
}

function getTopicFilters(request: Request) {
  const searchParams = new URL(request.url).searchParams
  const qaStatusValue = searchParams.get("qaStatus") || ""
  const sortValue = searchParams.get("sort") || ""

  return {
    qaStatus: qaStatusOptions.includes(qaStatusValue) ? qaStatusValue : "",
    sort: sortOptions.includes(sortValue) ? sortValue : "",
  }
}

async function loadCategory(
  request: Request | undefined,
  categoryId: number
): Promise<Category | null> {
  if (categoryId <= 0) return null
  return apiFetch<Category>("/api/topic/category", {
    request,
    params: { categoryId },
  }).catch(() => null)
}

async function loadTag(
  request: Request | undefined,
  tagId: string | number | undefined
): Promise<Tag | null> {
  if (!tagId) return null
  return apiFetch<Tag>(`/api/tag/${tagId}`, { request }).catch(() => null)
}

async function getCategoryFilters({
  request,
  categoryId,
}: {
  request?: Request
  categoryId: number
}) {
  if (!request || categoryId <= 0) return {}

  const filters = getTopicFilters(request)
  const category = await loadCategory(request, categoryId)
  if (category?.type === "qa") {
    return { qaStatus: filters.qaStatus }
  }
  if (category) {
    return { sort: filters.sort || "latestEdit" }
  }
  return {}
}

export async function loadCategories(request?: Request) {
  return apiFetch<Category[]>("/api/topic/category_navs", { request }).catch(
    () => []
  )
}

export async function loadTopics(params: {
  request?: Request
  cursor?: string
  categoryId?: string | number
  tagId?: string | number
  qaStatus?: string
  sort?: string
  roleName?: TopicRoleName | ""
  roleAfter?: string | number
  userIds?: string[]
}) {
  const path = params.tagId ? "/api/topic/tag/topics" : "/api/topic/topics"

  return apiFetch<PageData<Topic>>(path, {
    request: params.request,
    signal: params.request?.signal,
    params: {
      cursor: params.cursor || "",
      categoryId: params.categoryId,
      tagId: params.tagId,
      qaStatus: params.qaStatus,
      sort: params.sort,
      roleName: params.roleName,
      roleAfter: params.roleAfter,
      userIds: params.userIds?.join(","),
    },
  })
}

export async function loadTopicListRouteData(
  request?: Request
): Promise<TopicListRouteData> {
  const roleName = request
    ? normalizeTopicRoleName(
        new URL(request.url).searchParams.get(TOPIC_ROLE_NAME_PARAM)
      )
    : ""
  const roleAfter = roleName
    ? new URL(request?.url || "http://local").searchParams.get("roleAfter") ||
      ""
    : ""
  const [topics, categories] = await Promise.all([
    loadTopics({ request, roleName, roleAfter }),
    loadCategories(request),
  ])
  return { topics, categories, roleName, roleAfter }
}

export async function loadDailyReportRouteData(
  request?: Request,
  initialCategory?: Category
): Promise<DailyReportRouteData> {
  const filters = parseDailyReportFilters(
    new URL(request?.url || "http://local").searchParams
  )
  const category =
    initialCategory ||
    findDailyReportCategory(
      await apiFetch<Category[]>("/api/topic/categories", {
        request,
        signal: request?.signal,
      })
    )
  if (!category) {
    return {
      category: null,
      categories: [],
      authors: [],
      filters,
      topics: { results: [], cursor: "", hasMore: false },
    }
  }
  const [topics, authors] = await Promise.all([
    loadTopics({ request, categoryId: category.id, ...filters }),
    apiFetch<UserSummary[]>("/api/topic/authors", {
      request,
      signal: request?.signal,
      params: { categoryId: category.id },
    }),
  ])
  return { category, categories: [], topics, authors, filters }
}

export async function loadCategoryRouteData({
  request,
  id,
}: {
  request?: Request
  id?: string
}): Promise<TopicListRouteData> {
  if (id === "feed") {
    const user = await getCurrentUser(request)
    if (!user) {
      throw redirect("/user/signin?redirect=/topics/category/feed")
    }
  }

  const categoryId = resolveCategoryId(id)
  const category = await loadCategory(request, categoryId)
  if (category?.name === DAILY_REPORT_CATEGORY_NAME) {
    return loadDailyReportRouteData(request, category)
  }
  const roleName =
    categoryId === 0
      ? normalizeTopicRoleName(
          new URL(request?.url || "http://local").searchParams.get(
            TOPIC_ROLE_NAME_PARAM
          )
        )
      : ""
  const roleAfter = roleName
    ? new URL(request?.url || "http://local").searchParams.get("roleAfter") ||
      ""
    : ""
  const filters = await getCategoryFilters({ request, categoryId })
  const [topics, categories] = await Promise.all([
    loadTopics({ request, categoryId, roleName, roleAfter, ...filters }),
    loadCategories(request),
  ])
  return { topics, categories, category, roleName, roleAfter }
}

export async function loadTopicTagRouteData({
  request,
  id,
}: {
  request?: Request
  id?: string
}): Promise<TopicListRouteData> {
  const [topics, categories, tag] = await Promise.all([
    loadTopics({ request, tagId: id }),
    loadCategories(request),
    loadTag(request, id),
  ])
  return { topics, categories, tag }
}

export async function loader({
  request,
  params,
}: {
  request: Request
  params?: { id?: string }
}) {
  const pathname = new URL(request.url).pathname

  if (pathname === "/") {
    return loadDailyReportRouteData(request)
  }
  if (pathname === "/topics") {
    return loadTopicListRouteData(request)
  }
  if (pathname.startsWith("/topics/category/")) {
    return loadCategoryRouteData({ request, id: params?.id })
  }
  if (pathname.startsWith("/topics/tag/")) {
    return loadTopicTagRouteData({ request, id: params?.id })
  }
  if (pathname === "/articles") {
    return loadArticles({ request })
  }
  if (pathname.startsWith("/articles/tag/")) {
    return loadArticleListRouteData({ request, tagId: params?.id })
  }

  return null
}

export async function loadArticles(params: {
  request?: Request
  cursor?: string
  tagId?: string | number
}) {
  const path = params.tagId
    ? "/api/article/tag/articles"
    : "/api/article/articles"

  return apiFetch<PageData<Article>>(path, {
    request: params.request,
    params: {
      cursor: params.cursor || "",
      tagId: params.tagId,
    },
  })
}

export type ArticleListRouteData = PageData<Article> & {
  tag?: Tag | null
}

export async function loadArticleListRouteData(params: {
  request?: Request
  cursor?: string
  tagId?: string | number
}): Promise<ArticleListRouteData> {
  const [articles, tag] = await Promise.all([
    loadArticles(params),
    params.tagId
      ? loadTag(params.request, params.tagId)
      : Promise.resolve(null),
  ])
  return Object.assign(articles, { tag })
}

export async function loadTopicDetail(params: {
  request?: Request
  id: string | number
}) {
  return apiFetch<Topic>(`/api/topic/${params.id}`, {
    request: params.request,
  })
}

export async function loadArticleDetail(params: {
  request?: Request
  id: string | number
}) {
  return apiFetch<Article>(`/api/article/${params.id}`, {
    request: params.request,
  })
}
