package com.glitchcraft.likecounter;

import java.util.Optional;
import org.springframework.jdbc.core.simple.JdbcClient;
import org.springframework.stereotype.Service;

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

    public LikeResponse likePost(String postId, String userId) {
        Integer existing = jdbcClient.sql("""
                        SELECT COUNT(*)
                        FROM post_likes
                        WHERE post_id = :postId AND user_id = :userId
                        """)
                .param("postId", postId)
                .param("userId", userId)
                .query(Integer.class)
                .single();

        if (existing > 0) {
            return new LikeResponse(postId, userId, false, false);
        }

        jdbcClient.sql("INSERT INTO post_likes (post_id, user_id) VALUES (:postId, :userId)")
                .param("postId", postId)
                .param("userId", userId)
                .update();

        jdbcClient.sql("UPDATE posts SET like_count = like_count + 1 WHERE id = :postId")
                .param("postId", postId)
                .update();

        return new LikeResponse(postId, userId, true, false);
    }

    public LikeResponse unlikePost(String postId, String userId) {
        jdbcClient.sql("DELETE FROM post_likes WHERE post_id = :postId AND user_id = :userId")
                .param("postId", postId)
                .param("userId", userId)
                .update();

        jdbcClient.sql("UPDATE posts SET like_count = like_count - 1 WHERE id = :postId")
                .param("postId", postId)
                .update();

        return new LikeResponse(postId, userId, false, true);
    }
}
