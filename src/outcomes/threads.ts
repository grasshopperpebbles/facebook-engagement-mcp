import type { Comment, PagePost } from "../vendor/meta-client/index.js"

export interface ThreadPost {
  id: string
  /**
   * False for unpublished, ad-backed posts; true for published ones.
   *
   * **Absent when it is not known.** Only a `/feed` sweep reads the post node,
   * so a post reached by id — a `post`, `ad` or `campaign` target — has no
   * published state to report, and an asserted default would mislabel a
   * boosted published post as ad-backed and an ad post fetched by id as
   * organic. Omission says "not checked"; it never means "published".
   */
  isPublished?: boolean
  message?: string
  permalink?: string
}

export interface Thread {
  comment: Comment
  replies: Comment[]
  post: ThreadPost
}

export function threadPost(post: PagePost): ThreadPost {
  return {
    id: post.id,
    isPublished: post.isPublished,
    ...(post.message !== undefined && { message: post.message }),
    ...(post.permalink !== undefined && { permalink: post.permalink }),
  }
}

export function assembleThreads(input: {
  post: ThreadPost
  comments: Comment[]
  repliesByCommentId: Map<string, Comment[]>
}): Thread[] {
  const { post } = input
  return input.comments.map((comment) => ({
    comment,
    replies: input.repliesByCommentId.get(comment.id) ?? [],
    post,
  }))
}
