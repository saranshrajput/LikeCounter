package com.glitchcraft.likecounter;

import java.util.Optional;
import org.springframework.http.HttpStatus;
import org.springframework.jdbc.core.simple.JdbcClient;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;
import org.springframework.web.server.ResponseStatusException;

@Service
public class PostLikeService {
    private final JdbcClient jdbcClient;

    public PostLikeService(JdbcClient jdbcClient) {
        this.jdbcClient = jdbcClient;
    }

    public Optional<PostResponse> getPost(String postId) {
        return jdbcClient.sql("SELECT id, title, like_count FROM posts WHERE id = :postId")
                .param("postId", postId)
                .query((rs, rowNum) -> new PostResponse(
                        rs.getString("id"),
                        rs.getString("title"),
                        rs.getInt("like_count")
                ))
                .optional();
    }

    /**
     * Idempotent like. The unique index on (post_id, user_id) decides who wins a race:
     * only the request whose INSERT actually created a row bumps the counter. Both
     * statements run in one transaction, so the row and the counter commit together.
     */
    @Transactional
    public LikeResponse likePost(String postId, String userId) {
        requireExists("posts", postId, "Post not found");
        requireExists("users", userId, "User not found");

        int inserted = jdbcClient.sql("""
                        INSERT INTO post_likes (post_id, user_id)
                        VALUES (:postId, :userId)
                        ON CONFLICT (post_id, user_id) DO NOTHING
                        """)
                .param("postId", postId)
                .param("userId", userId)
                .update();

        if (inserted == 1) {
            jdbcClient.sql("UPDATE posts SET like_count = like_count + 1 WHERE id = :postId")
                    .param("postId", postId)
                    .update();
        }

        return new LikeResponse(postId, userId, inserted == 1, false);
    }

    /**
     * Idempotent unlike. DELETE reports how many rows it removed; only a request that
     * really removed the row decrements, so the count can't drift or go negative.
     */
    @Transactional
    public LikeResponse unlikePost(String postId, String userId) {
        requireExists("posts", postId, "Post not found");

        int deleted = jdbcClient.sql("DELETE FROM post_likes WHERE post_id = :postId AND user_id = :userId")
                .param("postId", postId)
                .param("userId", userId)
                .update();

        if (deleted == 1) {
            jdbcClient.sql("UPDATE posts SET like_count = like_count - 1 WHERE id = :postId")
                    .param("postId", postId)
                    .update();
        }

        return new LikeResponse(postId, userId, false, deleted == 1);
    }

    // `table` is only ever a hard-coded literal from this class, never user input.
    private void requireExists(String table, String id, String message) {
        boolean exists = jdbcClient.sql("SELECT EXISTS (SELECT 1 FROM " + table + " WHERE id = :id)")
                .param("id", id)
                .query(Boolean.class)
                .single();
        if (!exists) {
            throw new ResponseStatusException(HttpStatus.NOT_FOUND, message);
        }
    }
}
