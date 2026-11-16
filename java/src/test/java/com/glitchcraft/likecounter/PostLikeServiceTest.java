package com.glitchcraft.likecounter;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.Callable;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.function.IntFunction;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.dao.DataIntegrityViolationException;
import org.springframework.jdbc.core.simple.JdbcClient;
import org.springframework.web.server.ResponseStatusException;

@SpringBootTest
class PostLikeServiceTest {
    private static final int USERS = 50;

    @Autowired
    private PostLikeService postLikeService;

    @Autowired
    private JdbcClient jdbcClient;

    @BeforeEach
    void resetDatabase() {
        jdbcClient.sql("TRUNCATE post_likes, posts, users RESTART IDENTITY CASCADE").update();
        // user-1 .. user-50
        jdbcClient.sql("INSERT INTO users (id, name) SELECT 'user-' || g, 'User ' || g FROM generate_series(1, " + USERS + ") g")
                .update();
        jdbcClient.sql("""
                INSERT INTO posts (id, title, like_count) VALUES
                    ('post-1', 'Scaling counters with Postgres', 0)
                """).update();
    }

    @Test
    void incrementsVisibleLikeCountWhenUserLikesPost() {
        LikeResponse response = postLikeService.likePost("post-1", "user-1");

        assertThat(response.created()).isTrue();
        assertThat(likeCount("post-1")).isEqualTo(1);
    }

    @Test
    void doesNotIncrementTwiceForSequentialDuplicateLikes() {
        LikeResponse first = postLikeService.likePost("post-1", "user-1");
        LikeResponse second = postLikeService.likePost("post-1", "user-1");

        assertThat(first.created()).isTrue();
        assertThat(second.created()).isFalse();
        assertThat(countPostLikes("post-1", "user-1")).isEqualTo(1);
        assertThat(likeCount("post-1")).isEqualTo(1);
    }

    @Test
    void doesNotDecrementWhenUserHasNotLikedPost() {
        LikeResponse response = postLikeService.unlikePost("post-1", "user-1");

        assertThat(response.deleted()).isFalse();
        assertThat(likeCount("post-1")).isZero();
    }

    @Test
    void unlikeRemovesLikeAndDecrementsOnce() {
        postLikeService.likePost("post-1", "user-1");

        assertThat(postLikeService.unlikePost("post-1", "user-1").deleted()).isTrue();
        assertThat(postLikeService.unlikePost("post-1", "user-1").deleted()).isFalse();
        assertThat(likeCount("post-1")).isZero();
    }

    @Test
    void handlesConcurrentDuplicateLikesFromSameUser() throws Exception {
        List<LikeResponse> results = runConcurrently(20, i -> () -> postLikeService.likePost("post-1", "user-1"));

        assertThat(results.stream().filter(LikeResponse::created)).hasSize(1);
        assertThat(countPostLikes("post-1", "user-1")).isEqualTo(1);
        assertThat(likeCount("post-1")).isEqualTo(1);
    }

    @Test
    void handlesConcurrentUnlikesFromSameUser() throws Exception {
        postLikeService.likePost("post-1", "user-1");

        List<LikeResponse> results = runConcurrently(20, i -> () -> postLikeService.unlikePost("post-1", "user-1"));

        assertThat(results.stream().filter(LikeResponse::deleted)).hasSize(1);
        assertThat(countPostLikes("post-1", "user-1")).isZero();
        assertThat(likeCount("post-1")).isZero();
    }

    @Test
    void handlesConcurrentUnlikesWhenUserNeverLiked() throws Exception {
        runConcurrently(20, i -> () -> postLikeService.unlikePost("post-1", "user-1"));

        assertThat(likeCount("post-1")).isZero();
    }

    @Test
    void handlesConcurrentLikesFromManyDistinctUsers() throws Exception {
        runConcurrently(USERS, i -> () -> postLikeService.likePost("post-1", "user-" + (i + 1)));

        assertThat(likeCount("post-1")).isEqualTo(USERS);
        assertThat(totalLikeRows("post-1")).isEqualTo(USERS);
    }

    @Test
    void keepsCounterEqualToRowsUnderMixedConcurrentLikesAndUnlikes() throws Exception {
        // 10 users get liked up front; then likes (some duplicated) and unlikes race.
        for (int u = 1; u <= 10; u++) {
            postLikeService.likePost("post-1", "user-" + u);
        }

        runConcurrently(200, i -> () -> {
            String user = "user-" + (i % USERS + 1);
            return i % 2 == 0 ? postLikeService.likePost("post-1", user) : postLikeService.unlikePost("post-1", user);
        });

        int rows = totalLikeRows("post-1");
        assertThat(likeCount("post-1")).isEqualTo(rows).isGreaterThanOrEqualTo(0);
    }

    @Test
    void returnsNotFoundForUnknownPostOrUser() {
        assertThatThrownBy(() -> postLikeService.likePost("nope", "user-1"))
                .isInstanceOf(ResponseStatusException.class).hasMessageContaining("404");
        assertThatThrownBy(() -> postLikeService.likePost("post-1", "ghost"))
                .isInstanceOf(ResponseStatusException.class).hasMessageContaining("404");
        assertThatThrownBy(() -> postLikeService.unlikePost("nope", "user-1"))
                .isInstanceOf(ResponseStatusException.class).hasMessageContaining("404");
    }

    @Test
    void databaseRejectsNegativeCounterAndDuplicateLikeRows() {
        assertThatThrownBy(() -> jdbcClient.sql("UPDATE posts SET like_count = -1 WHERE id = 'post-1'").update())
                .isInstanceOf(DataIntegrityViolationException.class);

        jdbcClient.sql("INSERT INTO post_likes (post_id, user_id) VALUES ('post-1', 'user-1')").update();
        assertThatThrownBy(() -> jdbcClient.sql("INSERT INTO post_likes (post_id, user_id) VALUES ('post-1', 'user-1')").update())
                .isInstanceOf(DataIntegrityViolationException.class);
    }

    private int likeCount(String postId) {
        return postLikeService.getPost(postId).orElseThrow().likeCount();
    }

    private int totalLikeRows(String postId) {
        return jdbcClient.sql("SELECT COUNT(*) FROM post_likes WHERE post_id = :postId")
                .param("postId", postId)
                .query(Integer.class)
                .single();
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

    /**
     * Runs {@code count} tasks on a dedicated pool, released together by a latch so they
     * genuinely overlap. Exceptions from any task fail the test (Future.get rethrows).
     */
    private static <T> List<T> runConcurrently(int count, IntFunction<Callable<T>> taskFactory) throws Exception {
        ExecutorService pool = Executors.newFixedThreadPool(count);
        try {
            CountDownLatch ready = new CountDownLatch(count);
            CountDownLatch go = new CountDownLatch(1);
            List<Future<T>> futures = new ArrayList<>();
            for (int i = 0; i < count; i++) {
                Callable<T> task = taskFactory.apply(i);
                futures.add(pool.submit(() -> {
                    ready.countDown();
                    go.await();
                    return task.call();
                }));
            }
            ready.await();
            go.countDown();
            List<T> results = new ArrayList<>();
            for (Future<T> f : futures) {
                results.add(f.get());
            }
            return results;
        } finally {
            pool.shutdownNow();
        }
    }
}
