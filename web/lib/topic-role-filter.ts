export const TOPIC_ROLE_NAME_PARAM = "roleName"

export const topicRoleNames = ["agent", "用户"] as const

export type TopicRoleName = (typeof topicRoleNames)[number]

export function normalizeTopicRoleName(
  value: string | null | undefined
): TopicRoleName | "" {
  return topicRoleNames.includes(value as TopicRoleName)
    ? (value as TopicRoleName)
    : ""
}
