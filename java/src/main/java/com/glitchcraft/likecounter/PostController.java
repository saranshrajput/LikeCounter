package com.glitchcraft.likecounter;

import jakarta.validation.Valid;
import jakarta.validation.constraints.NotBlank;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.DeleteMapping;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RestController;

@RestController
public class PostController {
    private final PostLikeService postLikeService;

    public PostController(PostLikeService postLikeService) {
        this.postLikeService = postLikeService;
    }

    @GetMapping("/health")
    public HealthResponse health() {
        return new HealthResponse(true);
    }

    @GetMapping("/posts/{postId}")
    public ResponseEntity<PostResponse> getPost(@PathVariable String postId) {
        return postLikeService.getPost(postId)
                .map(ResponseEntity::ok)
                .orElseGet(() -> ResponseEntity.notFound().build());
    }

    @PostMapping("/posts/{postId}/like")
    public ResponseEntity<LikeResponse> likePost(
            @PathVariable String postId,
            @Valid @RequestBody LikeRequest request
    ) {
        LikeResponse response = postLikeService.likePost(postId, request.userId());
        return ResponseEntity.status(response.created() ? HttpStatus.CREATED : HttpStatus.OK).body(response);
    }

    @DeleteMapping("/posts/{postId}/like")
    public LikeResponse unlikePost(
            @PathVariable String postId,
            @Valid @RequestBody LikeRequest request
    ) {
        return postLikeService.unlikePost(postId, request.userId());
    }

    public record HealthResponse(boolean ok) {
    }

    public record LikeRequest(@NotBlank String userId) {
    }
}
