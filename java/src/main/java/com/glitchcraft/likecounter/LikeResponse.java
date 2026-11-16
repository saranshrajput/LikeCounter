package com.glitchcraft.likecounter;

public record LikeResponse(String postId, String userId, boolean created, boolean deleted) {
}
