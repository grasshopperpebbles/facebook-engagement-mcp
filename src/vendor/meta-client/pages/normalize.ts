import type { Comment, Page, PagePost, RawComment, RawPage, RawPost } from "./types.js"

export function normalizePage(raw: RawPage): Page {
  return {
    id: raw.id,
    ...(raw.name !== undefined && { name: raw.name }),
    ...(raw.category !== undefined && { category: raw.category }),
    ...(raw.tasks !== undefined && { tasks: raw.tasks }),
  }
}

export function normalizePost(raw: RawPost): PagePost {
  return {
    id: raw.id,
    ...(raw.message !== undefined && { message: raw.message }),
    ...(raw.created_time !== undefined && { createdTime: raw.created_time }),
    ...(raw.permalink_url !== undefined && { permalink: raw.permalink_url }),
    isPublished: raw.is_published ?? true,
  }
}

export function normalizeComment(raw: RawComment): Comment {
  return {
    id: raw.id,
    ...(raw.message !== undefined && { message: raw.message }),
    ...(raw.created_time !== undefined && { createdTime: raw.created_time }),
    ...(raw.from !== undefined && { author: raw.from }),
    ...(raw.like_count !== undefined && { likeCount: raw.like_count }),
    ...(raw.comment_count !== undefined && { replyCount: raw.comment_count }),
    ...(raw.is_hidden !== undefined && { hidden: raw.is_hidden }),
    ...(raw.can_comment !== undefined && { canComment: raw.can_comment }),
    ...(raw.can_hide !== undefined && { canHide: raw.can_hide }),
    ...(raw.permalink_url !== undefined && { permalink: raw.permalink_url }),
    ...(raw.parent?.id !== undefined && { parentId: raw.parent.id }),
  }
}
