package com.glitchcraft.likecounter;

import static org.assertj.core.api.Assertions.assertThat;

import java.util.List;
import java.util.concurrent.CompletableFuture;
import java.util.stream.IntStream;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.jdbc.core.simple.JdbcClient;

@SpringBootTest
class PostLikeServiceTest {
    @Autowired
    private PostLikeService postLikeService;

    @Autowired
    private JdbcClient jdbcClient;

    @BeforeEach
    void resetDatabase() {
        jdbcClient.sql("TRUNCATE post_likes, posts, users RESTART IDENTITY CASCADE").update();
        jdbcClient.sql("""
                INSERT INTO users (id, name) VALUES
                    ('user-1', 'Ada Lovelace'),
                    ('user-2', 'Grace Hopper')
                """).update();
        jdbcClient.sql("""
                INSERT INTO posts (id, title, like_count) VALUES
                    ('post-1', 'Scaling counters with Postgres', 0)
                """).update();
    }

    @Test
    void incrementsVisibleLikeCountWhenUserLikesPost() {
        postLikeService.likePost("post-1", "user-1");

        assertThat(postLikeService.getPost("post-1"))
                .hasValueSatisfying(post -> assertThat(post.likeCount()).isEqualTo(1));
    }

    @Test
    void doesNotIncrementTwiceForSequentialDuplicateLikes() {
        postLikeService.likePost("post-1", "user-1");
        postLikeService.likePost("post-1", "user-1");

        assertThat(countPostLikes("post-1", "user-1")).isEqualTo(1);
        assertThat(postLikeService.getPost("post-1"))
                .hasValueSatisfying(post -> assertThat(post.likeCount()).isEqualTo(1));
    }

    @Test
    void doesNotDecrementWhenUserHasNotLikedPost() {
        postLikeService.unlikePost("post-1", "user-1");

        assertThat(postLikeService.getPost("post-1"))
                .hasValueSatisfying(post -> assertThat(post.likeCount()).isZero());
    }

    @Test
    void handlesConcurrentDuplicateLikesFromSameUser() {
        runConcurrently(20, () -> postLikeService.likePost("post-1", "user-1"));

        assertThat(countPostLikes("post-1", "user-1")).isEqualTo(1);
        assertThat(postLikeService.getPost("post-1"))
                .hasValueSatisfying(post -> assertThat(post.likeCount()).isEqualTo(1));
    }

    @Test
    void handlesConcurrentUnlikesFromSameUser() {
        postLikeService.likePost("post-1", "user-1");

        runConcurrently(20, () -> postLikeService.unlikePost("post-1", "user-1"));

        assertThat(countPostLikes("post-1", "user-1")).isZero();
        assertThat(postLikeService.getPost("post-1"))
                .hasValueSatisfying(post -> assertThat(post.likeCount()).isZero());
    }

    private int countPostLikes(String postId, String userId) {
        return jdbcClient.sql("""
                        SELECT COUNT(*)
                        FROM post_likes
                        WHERE post_id = :postId AND user_id = :userId
                        """)
                .param("postId", postId)
                .param("userId", userId)
                .query(Integer.class)
                .single();
    }

    private static void runConcurrently(int count, Runnable operation) {
        List<CompletableFuture<Void>> futures = IntStream.range(0, count)
                .mapToObj(ignored -> CompletableFuture.runAsync(operation))
                .toList();

        CompletableFuture.allOf(futures.toArray(CompletableFuture[]::new)).join();
    }
}
